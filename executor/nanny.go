package executor

import (
	"context"
	"os"
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
	last_pump_to_rb_attempt        time.Time
	last_pump_rb_to_server_attempt time.Time
	last_read_from_server          time.Time

	MaxMemoryHardLimit uint64
	MaxConnectionDelay time.Duration

	logger *logging.LogContext

	on_exit func()
}

func (self *NannyService) UpdatePumpToRb() {
	self.last_pump_to_rb_attempt = Clock.Now()
}

func (self *NannyService) UpdatePumpRbToServer() {
	self.last_pump_rb_to_server_attempt = Clock.Now()
}

func (self *NannyService) UpdateReadFromServer() {
	self.last_read_from_server = Clock.Now()
}

func (self *NannyService) CheckTime(t time.Time, message string) {
	now := Clock.Now()
	if t.Add(self.MaxConnectionDelay).Before(now) {
		self.logger.Error(
			"NannyService: <red>Last %v too long ago %v</>", message, t)
		self.Exit()
	}
}

func (self *NannyService) Exit() {
	if self.on_exit == nil {
		os.Exit(-1)
	}
	self.on_exit()
}

func (self *NannyService) Start(
	ctx context.Context,
	wg *sync.WaitGroup) {

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer self.logger.Info("<red>Exiting</> nanny")

		self.logger.Info("<green>Starting</> nanny")

		for {
			select {
			case <-ctx.Done():
				return

			case <-Clock.After(10 * time.Second):
				self.CheckTime(self.last_pump_to_rb_attempt, "Pump to Ring Buffer")
				self.CheckTime(self.last_pump_rb_to_server_attempt, "Pump Ring Buffer to Server")
				self.CheckTime(self.last_read_from_server, "Read From Server")
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
		logger: logging.GetLogger(config_obj, &logging.ClientComponent),
	}

	Nanny.Start(ctx, wg)
	return nil
}
