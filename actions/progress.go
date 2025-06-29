package actions

import (
	"bytes"
	"context"
	"runtime/pprof"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type ProgressThrottler struct {
	mu               sync.Mutex
	delegate         types.Throttler
	progress_timeout time.Duration
	heartbeat        time.Time

	// Will be called when an alarm is fired.
	cancel func()
}

func (self *ProgressThrottler) ChargeOp() {
	self.mu.Lock()
	self.heartbeat = utils.Now()
	self.mu.Unlock()
	self.delegate.ChargeOp()
}

func (self *ProgressThrottler) Close() {
	self.delegate.Close()
}

func (self *ProgressThrottler) Start(
	ctx context.Context, scope vfilter.Scope) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(self.progress_timeout):
			self.mu.Lock()
			now := utils.Now()
			if self.progress_timeout.Nanoseconds() > 0 &&
				now.After(self.heartbeat.Add(self.progress_timeout)) {
				self.mu.Unlock()
				self.exitWithError(scope)
				return
			}
			self.mu.Unlock()
		}
	}
}

func (self *ProgressThrottler) exitWithError(scope vfilter.Scope) {
	scope.Log("ERROR:No progress made in %v seconds! aborting.",
		self.progress_timeout)

	buf := bytes.Buffer{}
	p := pprof.Lookup("goroutine")
	if p != nil {
		_ = p.WriteTo(&buf, 1)
		scope.Log("Goroutine dump: %v", buf.String())
	}

	buf = bytes.Buffer{}
	p = pprof.Lookup("mutex")
	if p != nil {
		_ = p.WriteTo(&buf, 1)
		scope.Log("Mutex dump: %v", buf.String())
	}

	for _, q := range QueryLog.Get() {
		scope.Log("Recent Query: %v", q)
	}
	self.cancel()
}

func NewProgressThrottler(
	ctx context.Context, scope vfilter.Scope,
	cancel func(),
	throttler types.Throttler,
	progress_timeout time.Duration) types.Throttler {
	result := &ProgressThrottler{
		cancel:           cancel,
		delegate:         throttler,
		progress_timeout: progress_timeout,
	}

	go result.Start(ctx, scope)
	return result
}
