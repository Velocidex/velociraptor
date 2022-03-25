package executor

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	Nanny             = &NannyService{}
	Clock utils.Clock = utils.RealClock{}
)

type NannyService struct {
	mu                             sync.Mutex
	last_pump_to_rb_attempt        time.Time
	last_pump_rb_to_server_attempt time.Time
	last_read_from_server          time.Time

	MaxMemoryHardLimit uint64
	MaxConnectionDelay time.Duration

	Logger *logging.LogContext

	// Function that will be called when the nanny detects out of
	// specs condition. If not specified we exit immediately.
	OnExit func()

	on_exit_called bool
}

func (self *NannyService) UpdatePumpToRb() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_pump_to_rb_attempt = Clock.Now()
}

func (self *NannyService) UpdatePumpRbToServer() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_pump_rb_to_server_attempt = Clock.Now()
}

func (self *NannyService) UpdateReadFromServer() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_read_from_server = Clock.Now()
}

func (self *NannyService) _CheckMemory(message string) bool {
	if self.MaxMemoryHardLimit == 0 {
		return false
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if m.Alloc > self.MaxMemoryHardLimit {
		self.Logger.Error(
			"NannyService: <red>%v of %v bytes: current heap usage %v bytes</>",
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

	now := Clock.Now()
	if t.Add(self.MaxConnectionDelay).Before(now) {
		self.Logger.Error(
			"NannyService: <red>Last %v too long ago %v</>", message, t)
		self._Exit()
		return true
	}

	return false
}

func (self *NannyService) _Exit() {
	// If we already called the OnExit one time, we just hard exit.
	if self.OnExit == nil || self.on_exit_called {
		self.Logger.Error("Hard Exit called!")
		os.Exit(-1)
		return
	}

	on_exit := self.OnExit
	self.mu.Unlock()
	// Release the lock in case the on exit function needs to send things.
	on_exit()
	self.mu.Lock()
	self.on_exit_called = true
}

func (self *NannyService) Start(
	ctx context.Context,
	wg *sync.WaitGroup) {

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer self.Logger.Info("<red>Exiting</> nanny")

		self.Logger.Info("<green>Starting</> nanny")

		for {
			select {
			case <-ctx.Done():
				return

			case <-Clock.After(10 * time.Second):
				self.mu.Lock()
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
				self.mu.Unlock()
			}
		}
	}()
}

func StartNannyService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	if config_obj.Client == nil {
		return nil
	}

	Nanny = &NannyService{
		MaxMemoryHardLimit: config_obj.Client.MaxMemoryHardLimit,
		MaxConnectionDelay: time.Duration(5*config_obj.Client.MaxPoll) *
			time.Second,
		Logger: logging.GetLogger(config_obj, &logging.ClientComponent),
	}

	Nanny.Start(ctx, wg)
	return nil
}
