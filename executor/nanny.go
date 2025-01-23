package executor

import (
	"context"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	Nanny = &NannyService{}
)

type NannyService struct {
	mu                             sync.Mutex
	last_pump_to_rb_attempt        time.Time
	last_pump_rb_to_server_attempt time.Time
	last_read_from_server          time.Time
	last_check_time                time.Time

	MaxMemoryHardLimit uint64
	MaxConnectionDelay time.Duration

	Logger *logging.LogContext

	// Function that will be called when the nanny detects out of
	// specs condition. If not specified we exit immediately.  This
	// function will only be called once. If the exit condition occurs
	// further OnExit2 will be called repeatadly.
	OnExit  func()
	OnExit2 func()

	on_exit_called bool
}

func (self *NannyService) UpdatePumpToRb() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_pump_to_rb_attempt = utils.GetTime().Now()
}

func (self *NannyService) UpdatePumpRbToServer() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_pump_rb_to_server_attempt = utils.GetTime().Now()
}

func (self *NannyService) UpdateReadFromServer() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_read_from_server = utils.GetTime().Now()
}

func (self *NannyService) _CheckMemory(message string) bool {
	// We need to make sure our memory footprint is as
	// small as possible. The Velociraptor client
	// prioritizes low memory footprint over latency. We
	// just sent data to the server and we wont need that
	// for a while so we can free our memory to the OS.
	debug.FreeOSMemory()

	if self.MaxMemoryHardLimit == 0 {
		return false
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if m.Alloc > self.MaxMemoryHardLimit {
		self.Logger.Error(
			"NannyService: <red>Exceeding memory limit: %v of %v bytes: current heap usage %v bytes</>",
			message, self.MaxMemoryHardLimit, m.Alloc)

		self._Exit()
		return true
	}
	return false
}

func (self *NannyService) _CheckTime(t time.Time, message string) bool {
	// If the time is not updated yet wait until it gets first
	// updated.
	if t.IsZero() {
		return false
	}

	now := utils.GetTime().Now()
	if t.Add(self.MaxConnectionDelay).Before(now) {
		self.Logger.Error(
			"NannyService: <red>Last %v too long ago %v (now is %v MaxConnectionDelay is %v)</>",
			message, t, now, self.MaxConnectionDelay)
		self._Exit()
		return true
	}

	return false
}

func (self *NannyService) _Exit() {
	// If we already called the OnExit one time, we just hard exit.
	if self.OnExit == nil || self.on_exit_called {
		self.Logger.Error("Hard Exit called!")
		if self.OnExit2 != nil {
			self.OnExit2()
		} else {
			os.Exit(-1)
		}
		return
	}

	on_exit := self.OnExit
	self.mu.Unlock()
	// Release the lock in case the on exit function needs to send things.
	on_exit()
	self.mu.Lock()
	self.on_exit_called = true
}

func (self *NannyService) checkOnce(period time.Duration) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Keep track of the last time we checked things.
	last_check_time := self.last_check_time
	self.last_check_time = utils.GetTime().Now()

	// We should update self.last_check_time periodically
	// so it shoud never be more than 20 sec behind this
	// check. Unless the machine just wakes up from sleep
	// - in that case the last check time is far before
	// this check time. In this case we cosider the check
	// invalid and try again later.
	if last_check_time.Add(period * 2).
		Before(self.last_check_time) {
		return
	}

	called := self._CheckTime(self.last_pump_to_rb_attempt, "Pump to Ring Buffer")
	if self._CheckTime(self.last_pump_rb_to_server_attempt, "Pump Ring Buffer to Server") {
		called = true
	}
	if self._CheckTime(self.last_read_from_server, "Read From Server") {
		called = true
	}
	if self._CheckMemory("Exceeded HardMemoryLimit") {
		called = true
	}

	// Allow the trigger to be disarmed if the on_exit was
	// able to reduce memory use or unstick the process.
	if !called {
		self.on_exit_called = false
	}
}

func (self *NannyService) Start(
	ctx context.Context,
	wg *sync.WaitGroup) {

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer self.Logger.Info("<red>Exiting</> nanny")

		self.Logger.Info(
			"<green>Starting</> nanny with MaxConnectionDelay %v and MaxMemoryHardLimit %v",
			self.MaxConnectionDelay, self.MaxMemoryHardLimit)

		period := 10 * time.Second

		for {
			select {
			case <-ctx.Done():
				return

			case <-utils.GetTime().After(period):
				self.checkOnce(period)
			}
		}
	}()
}

func NewNanny(
	config_obj *config_proto.Config) *NannyService {
	return &NannyService{
		MaxMemoryHardLimit: config_obj.Client.MaxMemoryHardLimit,
		last_check_time:    utils.GetTime().Now(),
		MaxConnectionDelay: time.Duration(
			config_obj.Client.NannyMaxConnectionDelay) * time.Second,
		Logger: logging.GetLogger(config_obj, &logging.ClientComponent),
	}
}

func StartNannyService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	if config_obj.Client == nil {
		return nil
	}

	Nanny = NewNanny(config_obj)
	if config_obj.Client.NannyMaxConnectionDelay > 0 {
		Nanny.Start(ctx, wg)
	}
	return nil
}
