package executor

import (
	"testing"
	"time"

	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type OnExitHelper struct {
	exit_called time.Time
}

func (self *OnExitHelper) Exit() {
	// Only record the first time we were called
	if self.exit_called.IsZero() {
		self.exit_called = utils.GetTime().Now()
	}
}

func TestNanny(t *testing.T) {
	period := 10 * time.Second

	// If we did not communicate with the server in 60 sec, hard exit.
	config_obj := config.GetDefaultConfig()
	config_obj.Client.NannyMaxConnectionDelay = 60

	closer := utils.MockTime(utils.NewMockClock(time.Unix(1000, 0)))
	defer closer()

	helper := OnExitHelper{}
	Nanny := NewNanny(config_obj)
	Nanny.OnExit = helper.Exit
	Nanny.OnExit2 = helper.Exit

	// Set all checks to now.
	Nanny.UpdatePumpToRb()
	Nanny.UpdatePumpRbToServer()
	Nanny.UpdateReadFromServer()

	// A check at time 1000 - should be fine as this is the same time
	// the pumps were touched.
	Nanny.checkOnce(period)

	// Move the time 70 sec on, 10 sec at the time. This emulates the
	// nanny periodic checking as happens at runtime.
	for i := 1000; i <= 1080; i += 10 {
		utils.MockTime(utils.NewMockClock(time.Unix(int64(i), 0)))
		Nanny.checkOnce(period)
	}

	// Only the first check after the 60 second timeout will trigger
	// an exit. Earlier checks will not trigger exit.
	assert.Equal(t, int64(1070), helper.exit_called.Unix())
}

// Check that nanny is able to detect a large time step (like endpoint
// sleep/suspend cycle)
func TestNannySleep(t *testing.T) {
	period := 10 * time.Second

	// If we did not communicate with the server in 60 sec, hard exit.
	config_obj := config.GetDefaultConfig()
	config_obj.Client.NannyMaxConnectionDelay = 60

	closer := utils.MockTime(utils.NewMockClock(time.Unix(1000, 0)))
	defer closer()

	helper := OnExitHelper{}
	Nanny := NewNanny(config_obj)
	Nanny.OnExit = helper.Exit
	Nanny.OnExit2 = helper.Exit

	// Set all checks to now.
	Nanny.UpdatePumpToRb()
	Nanny.UpdatePumpRbToServer()
	Nanny.UpdateReadFromServer()

	// A check at time 1000 - should be fine as this is the same time
	// the pumps were touched.
	Nanny.checkOnce(period)

	// Now emulate a suspend cycle - the next check occurs a long time
	// after the last check
	utils.MockTime(utils.NewMockClock(time.Unix(2000, 0)))

	Nanny.checkOnce(period)

	// Did not trigger an exit.
	assert.True(t, helper.exit_called.IsZero())

	// Step the next check by 10 sec
	utils.MockTime(utils.NewMockClock(time.Unix(2010, 0)))

	// This will now trigger an exit
	Nanny.checkOnce(period)

	// First check after the 60 second timeout will trigger an exit.
	assert.Equal(t, int64(2010), helper.exit_called.Unix())
}
