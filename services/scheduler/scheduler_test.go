package scheduler_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils/rand"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type SchedulerTestSuite struct {
	test_utils.TestSuite
}

func (self *SchedulerTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true
	self.ConfigObj.Services.ApiServer = true
	self.TestSuite.SetupTest()
}

func (self *SchedulerTestSuite) TestScheduler() {
	defer rand.DisableRand()

	scheduler, err := services.GetSchedulerService(self.ConfigObj)
	assert.NoError(self.T(), err)

	received_jobs := &bytes.Buffer{}

	// Register a worker
	ctx, cancel := context.WithCancel(self.Ctx)
	out, err := scheduler.RegisterWorker(ctx, "Foobar", "Name Worker", 10)
	assert.NoError(self.T(), err)

	// Now read worker jobs in the background
	go func() {
		defer cancel()

		// Read a single job
		job := <-out
		_, err := received_jobs.Write([]byte(job.Job))
		job.Done("", err)
	}()

	// Make a call to the worker through the scheduler.
	res_chan, err := scheduler.Schedule(self.Ctx, services.SchedulerJob{
		Queue: "Foobar",
		Job:   "Hello world",
	})
	assert.NoError(self.T(), err)

	result := <-res_chan
	assert.NoError(self.T(), result.Err)
	assert.Contains(self.T(), string(received_jobs.Bytes()), "Hello world")

	// The schedule had now exited so we can not schedule any more
	_, err = scheduler.Schedule(self.Ctx, services.SchedulerJob{
		Queue: "Foobar",
		Job:   "Hello world 2",
	})
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "No workers available")
}

func TestScheduler(t *testing.T) {
	suite.Run(t, &SchedulerTestSuite{})
}
