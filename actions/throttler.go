/*
  Limit query progress to reduce overall CPU and IOPS usage to
  acceptable levels.
*/

package actions

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/process"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	throttlerChargeOpCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vql_throttler_charge_op_counter",
		Help: "Counts the number of times the throttler checked the load.",
	})

	throttlerUpdateStatsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vql_throttler_update_stats_counter",
		Help: "Counts the number of times the throttler updated the stats.",
	})

	throttlerCurrentGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "vql_throttler_current",
		Help: "Counts the number of throttlers currently available.",
	})

	stats *statsCollector
)

type sample struct {
	total_cpu_usage  float64
	average_cpu_load float64
	timestamp_ms     float64
}

type statsCollector struct {
	mu   sync.Mutex
	cond *sync.Cond
	id   uint64

	proc *process.Process

	samples [2]sample

	// Count of how many throttlers are watching this stats collector.
	waiters int

	// How often to check the cpu load
	check_duration_msec float64
	number_of_cores     float64
}

func newStatsCollector() (*statsCollector, error) {
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil || proc == nil {
		return nil, err
	}

	number_of_cores, err := cpu.Counts(true)
	if err != nil || number_of_cores <= 0 {
		return nil, err
	}

	result := &statsCollector{
		check_duration_msec: 300,
		id:                  utils.GetId(),
		proc:                proc,
		number_of_cores:     float64(number_of_cores),
	}
	result.cond = sync.NewCond(&result.mu)

	now := float64(time.Now().UnixNano()) / 1000000
	result.samples[1].timestamp_ms = now - result.check_duration_msec
	result.updateStats(context.Background())

	// Start the stats collector
	go result.Start()

	return result, nil
}

func (self *statsCollector) Start() {
	// Make sure when we exit, the throttler is left in the open
	// state.
	defer func() {
		self.mu.Lock()
		self.samples[0] = sample{}
		self.samples[1] = sample{}
		self.cond.Broadcast()
		self.mu.Unlock()
	}()

	ctx := context.Background()
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

// Estimate the current CPU load by first order derivative.
func (self *statsCollector) GetAverageCPULoad() float64 {
	return self.samples[1].average_cpu_load
}

// This returns the total number of CPU seconds used in total for the
// process. This is called not that frequently in order to minimize
// the overheads of making a system call.
func (self *statsCollector) getCpuTime(ctx context.Context) (float64, error) {
	cpu_time, err := self.proc.TimesWithContext(ctx)
	if err != nil {
		return 0, err
	}
	return cpu_time.Total(), nil
}

// This is called frequently to estimate the current CPU load.
func (self *statsCollector) updateStats(ctx context.Context) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.waiters <= 0 {
		return
	}

	throttlerUpdateStatsCounter.Inc()

	total_cpu_time, err := self.getCpuTime(ctx)
	if err != nil {
		return
	}
	now := float64(time.Now().UnixNano()) / 1000000

	last_sample := self.samples[1]

	// Calculate the % utilization since last sample.
	duration_msec := now - last_sample.timestamp_ms
	avg_cpu := (total_cpu_time - last_sample.total_cpu_usage) /
		(duration_msec / 1000) * 100 / self.number_of_cores

	// 100 point SMA of avg_cpu
	next_avg_cpu := last_sample.average_cpu_load +
		(avg_cpu-last_sample.average_cpu_load)/30

	next_sample := sample{
		total_cpu_usage:  total_cpu_time,
		average_cpu_load: next_avg_cpu,
		timestamp_ms:     now,
	}

	self.samples[0] = last_sample
	self.samples[1] = next_sample

	self.cond.Broadcast()
}

// This throttler allows the VQL query to run as long as the process
// CPU usage remains within the required levels. When average CPU
// usage is exceeded, the query is blocked.
type Throttler struct {
	// If the query allowed to run yet?
	cpu_load_limit float64
	stats          *statsCollector
}

// This function blocks if needed to limit overall CPU and IOPS
// averages
func (self *Throttler) ChargeOp() {
	throttlerChargeOpCounter.Inc()

	self.stats.mu.Lock()
	defer self.stats.mu.Unlock()

	// Wait here until we get unblocked.
	for self.cpu_load_limit > 0 &&
		self.stats.GetAverageCPULoad() > self.cpu_load_limit {
		self.stats.cond.Wait()
	}

}

func (self *Throttler) Close() {}

type DummyThrottler struct{}

func (self DummyThrottler) ChargeOp() {}
func (self DummyThrottler) Close()    {}

func NewThrottler(
	ctx context.Context,
	ops_per_sec, cpu_percent, iop_percent float64) types.Throttler {

	if cpu_percent == 0 {
		return &DummyThrottler{}
	}

	mu.Lock()
	defer mu.Unlock()

	if stats == nil {
		var err error
		stats, err = newStatsCollector()
		if err != nil {
			return nil
		}
	}

	stats.mu.Lock()
	throttlerCurrentGauge.Inc()
	stats.waiters++
	stats.mu.Unlock()

	go func() {
		<-ctx.Done()
		stats.mu.Lock()
		throttlerCurrentGauge.Dec()
		stats.waiters--
		stats.mu.Unlock()
	}()

	return &Throttler{cpu_load_limit: cpu_percent, stats: stats}
}
