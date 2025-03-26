package throttler

import (
	"context"
	"runtime"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// A Global stats collector is always running. When throttlers
	// register with it they can read the data.
	mu    sync.Mutex
	stats *statsCollector = &statsCollector{
		throttlers: make(map[string]*Throttler),
	}
)

type Options struct {
	CpuLimit, IOPsLimit bool
}

// Stores a sample of system performance. We calculate a Simple Moving
// Average (SMA) of all load metrics so we can throttle based on them.
type sample struct {
	total_cpu_usage  float64
	total_iops       float64
	average_cpu_load float64
	average_iops     float64
	timestamp_ms     float64
}

type statsCollector struct {
	mu sync.Mutex

	// The lifetime context of the collector
	ctx  context.Context
	cond *sync.Cond
	id   uint64

	// We keep 2 samples - the last measurement and the next
	// measurement. To calculate the SMA we shift from the current to
	// the last.
	samples [2]sample

	// How often to check the cpu load
	check_duration_msec float64
	number_of_cores     float64
	sample_count        int64

	// Registered throttlers
	throttlers map[string]*Throttler

	cpu_reporter  *CPUReporter
	iops_reporter *IOPSReporter

	opts Options
}

func StartStatsCollectorService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	result := &statsCollector{
		ctx:                 ctx,
		check_duration_msec: 300,
		id:                  utils.GetId(),
		number_of_cores:     float64(runtime.NumCPU()),
		cpu_reporter:        NewCPUReporter(),
		iops_reporter:       NewIOPSReporter(),
		throttlers:          make(map[string]*Throttler),
	}
	result.cond = sync.NewCond(&result.mu)

	now := float64(utils.Now().UnixNano()) / 1000000
	result.samples[1].timestamp_ms = now - result.check_duration_msec

	// Initialize the starts the first time.
	result.updateStats(ctx)

	// Start the stats collector
	wg.Add(1)
	go result.Start(ctx, wg)

	mu.Lock()
	stats = result
	mu.Unlock()

	return nil
}

func (self *statsCollector) recalcOptions() {
	res := Options{}
	for _, t := range self.throttlers {
		if t.Opts.CpuLimit {
			res.CpuLimit = true
		}

		if t.Opts.IOPsLimit {
			res.IOPsLimit = true
		}
	}

	self.opts = res
}

func (self *statsCollector) Register(t *Throttler) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.throttlers[utils.ToString(t.id)] = t
	self.recalcOptions()
}

func (self *statsCollector) Unregister(t *Throttler) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.throttlers, utils.ToString(t.id))
	self.recalcOptions()
}

// Start the collection loop.
func (self *statsCollector) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// Make sure when we exit, the throttler is left in the open
	// state.
	defer func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		self.samples[0] = sample{}
		self.samples[1] = sample{}
		self.cond.Broadcast()

		self.cpu_reporter.Close()
		self.iops_reporter.Close()

	}()

	// Start collecting stats
	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(
			time.Duration(self.check_duration_msec) * time.Millisecond):
			self.updateStats(ctx)
		}
	}
}

// Wait here until the callback is true.
func (self *statsCollector) Wait(ctx context.Context,
	cb func(stats *statsCollector) bool) int64 {

	// We are not active
	if self.cond == nil {
		return 0
	}

	// Report how long we wanted to run.
	var waited int64

	for {
		// Call the callback without lock held as it will call us back
		// to estimate CPU etc.
		if cb(self) {
			return waited
		}

		// Wait for the next update cycle
		now := utils.Now().UnixNano()
		self.mu.Lock()
		self.cond.Wait()
		self.mu.Unlock()

		waited += utils.Now().UnixNano() - now
	}
}

// Estimate the current CPU load by first order derivative.
func (self *statsCollector) GetAverageCPULoad() float64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.samples[1].average_cpu_load
}

func (self *statsCollector) GetAverageIOPS() float64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.samples[1].average_iops
}

// This is called frequently to estimate the current CPU load.
func (self *statsCollector) updateStats(ctx context.Context) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// No waiters we do not need to update the stats
	if len(self.throttlers) == 0 {
		return
	}

	self.sample_count++

	var total_cpu_time, iops float64

	// Only derive those lazily.
	if self.opts.CpuLimit {
		total_cpu_time = self.cpu_reporter.GetCpuTime(ctx)
	}

	// If no one is interested in iops we dont measure it.
	if self.opts.IOPsLimit {
		iops = self.iops_reporter.GetIops(ctx)
	}

	now := float64(utils.Now().UnixNano()) / 1000000

	last_sample := self.samples[1]

	// Calculate the % utilization since last sample.
	duration_msec := now - last_sample.timestamp_ms
	avg_cpu := (total_cpu_time - last_sample.total_cpu_usage) /
		(duration_msec / 1000) * 100 / self.number_of_cores

	// 30 point SMA of avg_cpu
	next_avg_cpu := last_sample.average_cpu_load +
		(avg_cpu-last_sample.average_cpu_load)/30

	avg_iops := (iops - last_sample.total_iops) /
		(duration_msec / 1000)

	next_avg_iops := last_sample.average_iops +
		(avg_iops-last_sample.average_iops)/30

	next_sample := sample{
		total_cpu_usage:  total_cpu_time,
		total_iops:       iops,
		average_cpu_load: next_avg_cpu,
		average_iops:     next_avg_iops,
		timestamp_ms:     now,
	}

	self.samples[0] = last_sample
	self.samples[1] = next_sample

	// Broadcast that the update cycle is done, throttlers will need
	// to check again if they can run.
	self.cond.Broadcast()
}
