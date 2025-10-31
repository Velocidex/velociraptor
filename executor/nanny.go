package executor

import (
	"context"
	"os"
	"runtime"
	os_debug "runtime/debug"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	Nanny = &NannyService{
		OnWarnings: make(map[uint64]func()),
	}
)

type NannyService struct {
	Ready bool

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

	// Called on First warning.
	OnWarnings map[uint64]func()

	// Called on hard exit.
	OnExit2 func()

	// Set when we are in the warning phase. Next compliance check
	// will call hard exit.
	warning bool
}

func (self *NannyService) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	now := utils.GetTime().Now()
	display := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return now.Sub(t).Round(time.Second).String()
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	output_chan <- ordereddict.NewDict().
		Set("Timestamp", now.UTC()).
		Set("WarningMode", self.warning).
		Set("LastPumpToRb", display(self.last_pump_to_rb_attempt)).
		Set("LastPumpToServer", display(self.last_pump_rb_to_server_attempt)).
		Set("LastReadFromServer", display(self.last_read_from_server)).
		Set("LastCheckTime", display(self.last_check_time)).
		Set("MaxMemoryHardLimit", self.MaxMemoryHardLimit).
		Set("CurrentMemory", m.Alloc).
		Set("MaxConnectionDelay",
			time.Duration(self.MaxConnectionDelay*time.Second).String())
}

func (self *NannyService) RegisterOnWarnings(id uint64, cb func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if cb == nil {
		delete(self.OnWarnings, id)
	} else {
		self.OnWarnings[id] = cb
	}
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
	os_debug.FreeOSMemory()

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

	t = reparseTime(t)
	now := utils.GetTime().Now()
	if t.Add(self.MaxConnectionDelay).Before(now) {
		self.Logger.Error(
			"NannyService: <red>Last %v too long ago %v (now is %v MaxConnectionDelay is %v)</>",
			message, t, now, self.MaxConnectionDelay)

		// Only exit on first attempt, otherwise record we fired.
		self._Exit()
		return true
	}

	return false
}

func (self *NannyService) _Exit() {
	// If we already called the OnWarnings one time, we just hard
	// exit.
	if self.warning {
		self.Logger.Error("Hard Exit called!")
		if self.OnExit2 != nil {
			self.OnExit2()
		} else {
			os.Exit(-1)
		}
		return
	}

	self.Logger.Debug("Nanny: <red>First warning...</> Next compliance check will result in hard exit")
	for _, cb := range self.OnWarnings {
		// Release the lock in case the warning function needs to send
		// things.
		self.mu.Unlock()
		cb()
		self.mu.Lock()
	}

	self.warning = true
}

// Golang handles time shift transparently so suspend should not
// affect time arithmetics. Here we want to fall back on the specific
// handling for time shifts so we can report it.
func reparseTime(t time.Time) time.Time {
	return time.Unix(0, t.UnixNano())
}

func (self *NannyService) checkOnce(period time.Duration) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Keep track of the last time we checked things.
	last_check_time := reparseTime(self.last_check_time)
	self.last_check_time = reparseTime(utils.GetTime().Now())

	// We should update self.last_check_time periodically
	// so it shoud never be more than 20 sec behind this
	// check. Unless the machine just wakes up from sleep
	// - in that case the last check time is far before
	// this check time. In this case we cosider the check
	// invalid and try again later.
	if last_check_time.Add(period * 2).Before(self.last_check_time) {
		self.Logger.Error(
			"NannyService: <red>Detected timeshift:</> last_check_time was %v which is %v ago",
			last_check_time.UTC().Format(time.RFC3339),
			self.last_check_time.Sub(last_check_time).
				Round(time.Second).String())

		// Reset all the times to compensate for the time shift
		self.last_pump_rb_to_server_attempt = self.last_check_time
		self.last_pump_to_rb_attempt = self.last_check_time
		self.last_read_from_server = self.last_check_time
		return
	}

	// Only call the exit function once - if it was called skip all
	// the other checks until the next round. Otherwise the other
	// times will be out of compliance as well and the exit function
	// will be called again in this round.
	if self._CheckTime(self.last_pump_to_rb_attempt,
		"Pump to Ring Buffer") {
		return
	}
	if self._CheckTime(self.last_pump_rb_to_server_attempt,
		"Pump Ring Buffer to Server") {
		return
	}
	if self._CheckTime(self.last_read_from_server, "Read From Server") {
		return
	}
	if self._CheckMemory("Exceeded HardMemoryLimit") {
		return
	}

	// Allow the trigger to be disarmed if the on_exit was able to
	// reduce memory use or unstick the process.
	if self.warning {
		self.Logger.Debug("NannyService: <green>Back in compliance</> - disarming")
	}
	self.warning = false
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
		Ready:              true,
		MaxMemoryHardLimit: config_obj.Client.MaxMemoryHardLimit,
		last_check_time:    utils.GetTime().Now(),
		MaxConnectionDelay: time.Duration(
			config_obj.Client.NannyMaxConnectionDelay) * time.Second,
		Logger:     logging.GetLogger(config_obj, &logging.ClientComponent),
		OnWarnings: make(map[uint64]func()),
	}
}

func StartNannyService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	// Allow at most a single functional nanny
	if config_obj.Client == nil || Nanny.Ready {
		return nil
	}

	Nanny = NewNanny(config_obj)
	if config_obj.Client.NannyMaxConnectionDelay > 0 {
		Nanny.Start(ctx, wg)

		debug.RegisterProfileWriter(debug.ProfileWriterInfo{
			Name:          "Nanny",
			Description:   "Inspect status of the nanny",
			ProfileWriter: Nanny.ProfileWriter,
			Categories:    []string{"Client"},
		})
	}
	return nil
}
