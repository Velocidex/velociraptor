/*
  Limit query progress to reduce overall CPU and IOPS usage to
  acceptable levels.
*/

package throttler

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This throttler allows the VQL query to run as long as the process
// CPU usage remains within the required levels. When average CPU
// usage is exceeded, the query is blocked.
type Throttler struct {
	// How long we waited to run.
	waited int64

	// Total number of times the throttler is blocked.
	total_blocked int64

	// Are we currently blocked?
	blocked int64

	ctx context.Context

	// If the query allowed to run yet?
	cpu_load_limit float64
	iops_limit     float64
	stats          *statsCollector
	id             uint64

	Opts Options

	started time.Time

	// The name of the query for debugging.
	query_name string
}

func (self *Throttler) Stats() *ordereddict.Dict {
	waited := atomic.LoadInt64(&self.waited)
	blocked := atomic.LoadInt64(&self.blocked)
	total_blocked := atomic.LoadInt64(&self.total_blocked)

	return ordereddict.NewDict().
		Set("Name", self.query_name).
		Set("WallClock", utils.GetTime().Now().
			Sub(self.started).Round(time.Second).String()).
		Set("CPULimit", self.cpu_load_limit).
		Set("IOPLimit", self.iops_limit).
		Set("TotalWait", time.Duration(waited).Round(time.Second).String()).
		Set("Blocked", blocked == 1).
		Set("TotalBlocked", total_blocked)
}

// Calculates when the throttler is allowed to run.
func (self *Throttler) CanRun(stats *statsCollector) bool {
	if self.cpu_load_limit > 0 &&
		stats.GetAverageCPULoad() > self.cpu_load_limit {
		return false
	}

	if self.iops_limit > 0 &&
		stats.GetAverageIOPS() > self.iops_limit {
		return false
	}

	return true
}

// This function blocks if needed to limit overall CPU and IOPS
// averages
func (self *Throttler) ChargeOp() {
	// Wait here until we can run.
	atomic.StoreInt64(&self.blocked, 1)

	waited := self.stats.Wait(self.ctx, self.CanRun)
	atomic.AddInt64(&self.waited, waited)
	if waited > 0 {
		atomic.AddInt64(&self.total_blocked, 1)
	}

	atomic.StoreInt64(&self.blocked, 0)
}

func (self *Throttler) Close() {}

// Used when not throttling is required.
type DummyThrottler struct{}

func (self DummyThrottler) ChargeOp() {}
func (self DummyThrottler) Close()    {}

func NewThrottler(
	ctx context.Context, scope vfilter.Scope,
	config_obj *config_proto.Config,
	ops_per_sec, cpu_percent, iops_limit float64) (types.Throttler, func()) {

	if ops_per_sec > 0 && ops_per_sec < 100 {
		cpu_percent = ops_per_sec
	}

	// cpu throttler can only work from 0 to 100%
	if cpu_percent < 0 || cpu_percent > 100 {
		scope.Log("Throttler: Cpu limit %v outside range 0-100 , ignoring\n", cpu_percent)
		cpu_percent = 0
	}

	// For pathologically slow systems we enforce a hard limit.
	if cpu_percent == 0 {
		low_resource_cpu_limit := uint64(2)
		if config_obj.Client != nil &&
			config_obj.Client.LowResourceCpuCount > 0 {
			low_resource_cpu_limit = config_obj.Client.LowResourceCpuCount
		}

		low_resource_max_cpu := uint64(50)
		if config_obj.Client != nil &&
			config_obj.Client.LowResourceMaxCpu > 0 {
			low_resource_max_cpu = config_obj.Client.LowResourceMaxCpu
		}

		if runtime.NumCPU() < int(low_resource_cpu_limit) {
			scope.Log("System has only one core: enforcing throttling")
			cpu_percent = float64(low_resource_max_cpu)
		}
	}

	// No throttling required
	if cpu_percent == 0 && iops_limit == 0 {
		return &DummyThrottler{}, func() {}
	}

	scope.Log("Will throttle query to %.0f percent of %.0f available CPU resources (%0.02f cores long term average).",
		cpu_percent, stats.number_of_cores,
		cpu_percent*stats.number_of_cores/100)

	res := &Throttler{
		ctx:            ctx,
		cpu_load_limit: cpu_percent,
		iops_limit:     iops_limit,
		stats:          stats,
		id:             utils.GetId(),
		started:        utils.GetTime().Now(),
		Opts: Options{
			CpuLimit:  cpu_percent > 0,
			IOPsLimit: iops_limit > 0,
		},
	}

	// Mark the throttler with some context.
	query_name_any, ok := scope.GetContext(constants.SCOPE_QUERY_NAME)
	if ok {
		res.query_name, _ = query_name_any.(string)
	}

	stats.Register(res)

	return res, func() {
		stats.Unregister(res)
	}
}

func init() {
	_ = prometheus.Register(promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "process_iops_count",
			Help: "Current IOPs level",
		}, stats.GetAverageIOPS))

	_ = prometheus.Register(promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "process_cpu_used",
			Help: "Total CPU utilization by this process",
		}, stats.GetAverageCPULoad))
}
