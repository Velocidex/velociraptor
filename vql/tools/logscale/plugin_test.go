package logscale

import (
	"testing"
	"time"

        "github.com/stretchr/testify/require"
        "github.com/stretchr/testify/suite"
)

type LogScalePluginTestSuite struct {
	suite.Suite

	args logscalePluginArgs
	queue *LogScaleQueue
}

func (self *LogScalePluginTestSuite) SetupTest() {
	self.args = logscalePluginArgs{
		ApiBaseUrl: validUrl,
		IngestToken: validAuthToken,
	}

	// config isn't used for these tests
	self.queue = NewLogScaleQueue(nil)
}

func (self *LogScalePluginTestSuite) TestValidateEmptyUrl() {
	self.args.ApiBaseUrl = ""
	err := self.args.validate()
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScalePluginTestSuite) TestValidateInvalidUrl() {
	self.args.ApiBaseUrl = "invalid-url"
	err := self.args.validate()
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScalePluginTestSuite) TestValidateValid() {
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateEmptyAuthToken() {
	self.args.IngestToken = ""
	err := self.args.validate()
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScalePluginTestSuite) TestValidateInvalidThreads() {
	self.args.Threads = -1
	err := self.args.validate()
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScalePluginTestSuite) TestValidateValidThreads() {
	self.args.Threads = validWorkerCount
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateSetEventBatchSizeValid() {
	self.args.EventBatchSize = 10
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateSetEventBatchSizeZero() {
	self.args.EventBatchSize = 0
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateSetEventBatchSizeNegative() {
	self.args.EventBatchSize = -10
	err := self.args.validate()
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScalePluginTestSuite) TestValidateSetBatchingTimeoutDurationValid() {
	self.args.BatchingTimeoutMs = 10
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateSetBatchingTimeoutDurationZero() {
	self.args.BatchingTimeoutMs = 0
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateSetBatchingTimeoutDurationNegative() {
	self.args.BatchingTimeoutMs = -10
	err := self.args.validate()
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScalePluginTestSuite) TestValidateHttpClientTimeoutDurationValid() {
	self.args.HttpTimeoutSec = 10
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateHttpClientTimeoutDurationZero() {
	self.args.HttpTimeoutSec = 0
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateHttpClientTimeoutDurationNegative() {
	self.args.HttpTimeoutSec = -10
	err := self.args.validate()
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScalePluginTestSuite) TestValidateStatsIntervalDurationValid() {
	self.args.StatsInterval = 10
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateStatsIntervalDurationZero() {
	self.args.StatsInterval = 0
	err := self.args.validate()
	require.NoError(self.T(), err)
}

func (self *LogScalePluginTestSuite) TestValidateStatsIntervalDurationNegative() {
	self.args.StatsInterval = -10
	err := self.args.validate()
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}


func (self *LogScalePluginTestSuite) CheckApply() {
	// URL and Token are assumed to be correct

	if self.args.Threads == 0 {
		require.Equal(self.T(), defaultNWorkers, self.queue.nWorkers)
	} else {
		require.Equal(self.T(), self.args.Threads, self.queue.nWorkers)
	}
	if self.args.BatchingTimeoutMs == 0 {
		require.Equal(self.T(), defaultBatchingTimeoutDuration, self.queue.batchingTimeoutDuration)
	} else {
		require.Equal(self.T(), time.Duration(self.args.BatchingTimeoutMs) * time.Millisecond, self.queue.batchingTimeoutDuration)
	}
	if self.args.HttpTimeoutSec == 0 {
		require.Equal(self.T(), defaultHttpClientTimeoutDuration, self.queue.httpClientTimeoutDuration)
	} else {
		require.Equal(self.T(), time.Duration(self.args.HttpTimeoutSec) * time.Second, self.queue.httpClientTimeoutDuration)
	}
	if self.args.EventBatchSize == 0 {
		require.Equal(self.T(), defaultEventBatchSize, self.queue.eventBatchSize)
	} else {
		require.Equal(self.T(), self.args.EventBatchSize, self.queue.eventBatchSize)
	}
	require.Equal(self.T(), self.args.Debug , self.queue.debug)
}

func (self *LogScalePluginTestSuite) TestApplyValid() {
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyValidThreads() {
	self.args.Threads = validWorkerCount
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyEventBatchSizeValid() {
	self.args.EventBatchSize = 10
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyEventBatchSizeZero() {
	self.args.EventBatchSize = 0
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyBatchingTimeoutDurationValid() {
	self.args.BatchingTimeoutMs = 10
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyBatchingTimeoutDurationZero() {
	self.args.BatchingTimeoutMs = 0
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyHttpClientTimeoutDurationValid() {
	self.args.HttpTimeoutSec = 10
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyHttpClientTimeoutDurationZero() {
	self.args.HttpTimeoutSec = 0
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyTagMapValid() {
	self.args.TagFields = []string{"x=y", "y=z", }
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.NotNil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyTagMapEmptyTagName() {
	self.args.TagFields = []string{"x=y", "=z", }
	err := applyArgs(&self.args, self.queue)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyTagMapMultipleEquals() {
	self.args.TagFields = []string{"x=y", "y=z=z", }
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.NotNil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyTagMapEmptyTagArg() {
	self.args.TagFields = []string{}
	err := applyArgs(&self.args, self.queue)
	require.NoError(self.T(), err)
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScalePluginTestSuite) TestApplyTagMapEmptyTagArgString() {
	self.args.TagFields = []string{"",}
	err := applyArgs(&self.args, self.queue)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
	self.CheckApply()
	require.Nil(self.T(), self.queue.tagMap)
}

func TestLogScalePlugin(t *testing.T) {
	gMaxPoll = 1
	gMaxPollDev = 1
        suite.Run(t, new(LogScalePluginTestSuite))
}
