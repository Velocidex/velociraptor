package client_info_test

import (
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *ClientInfoTestSuite) TestQueueMessages() {
	client_info_manager, err := services.GetClientInfoManager()
	assert.NoError(self.T(), err)

	message1 := &crypto_proto.VeloMessage{Source: "Server", SessionId: "1"}
	err = client_info_manager.QueueMessageForClient(self.client_id, message1, nil)
	assert.NoError(self.T(), err)

	manager := client_info_manager.(*client_info.ClientInfoManager)

	// Now retrieve all messages.
	tasks, err := manager.PeekClientTasks(self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(tasks))
	assert.True(self.T(), proto.Equal(tasks[0], message1))

	// We did not lease, so the tasks are not removed, but this
	// time we will lease.
	tasks, err = manager.GetClientTasks(self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 1)

	// No tasks available.
	tasks, err = manager.PeekClientTasks(self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 0)
}

func (self *ClientInfoTestSuite) TestFastQueueMessages() {
	client_info_manager, err := services.GetClientInfoManager()
	assert.NoError(self.T(), err)

	written := []*crypto_proto.VeloMessage{}

	for i := 0; i < 10; i++ {
		message := &crypto_proto.VeloMessage{Source: "Server", SessionId: fmt.Sprintf("%d", i)}
		err := client_info_manager.QueueMessageForClient(
			self.client_id, message, nil)
		assert.NoError(self.T(), err)

		written = append(written, message)
	}

	// Now retrieve all messages.
	tasks, err := client_info_manager.GetClientTasks(self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 10, len(tasks))

	// Does not have to return in sorted form.
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].SessionId < tasks[j].SessionId
	})

	for i := 0; i < 10; i++ {
		assert.True(self.T(), proto.Equal(tasks[i], written[i]))
	}
}

// Make sure we internally cache the client tasks list.
func (self *ClientInfoTestSuite) TestGetClientTasksIsCached() {
	client_info_manager, err := services.GetClientInfoManager()
	assert.NoError(self.T(), err)

	// Get metrics snapshot
	metric_name := "ClientTaskQueue_inf"
	snapshot := vtesting.GetMetrics(self.T(), metric_name)

	// Get the client tasks list once - the first time will hit the
	// datastore.
	tasks, err := client_info_manager.GetClientTasks(self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 0, len(tasks))
	metrics := vtesting.GetMetricsDifference(self.T(), ".", snapshot)
	client_queue_list_ops, _ := metrics.GetInt64(
		"datastore_latency__list_MemcacheDatastore_ClientTaskQueue_inf")
	assert.Equal(self.T(), int64(1), client_queue_list_ops)

	// Now hit it 100 more times
	for i := 0; i < 100; i++ {
		tasks, err := client_info_manager.GetClientTasks(self.client_id)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), 0, len(tasks))
	}

	// No more hits to the datastore.
	metrics = vtesting.GetMetricsDifference(self.T(), metric_name, snapshot)
	client_queue_list_ops, _ = metrics.GetInt64(
		"datastore_latency__list_MemcacheDatastore_ClientTaskQueue_inf")
	assert.Equal(self.T(), int64(1), client_queue_list_ops)

	// Schedule a new task for the client.
	err = client_info_manager.QueueMessageForClient(self.client_id,
		&crypto_proto.VeloMessage{}, func() {
			notifier := services.GetNotifier()
			notifier.NotifyListener(self.ConfigObj, self.client_id)
		})
	assert.NoError(self.T(), err)

	// Wait until we can see the new task
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		tasks, _ := client_info_manager.GetClientTasks(self.client_id)
		return len(tasks) == 1
	})

	// Check the metrics that we only actually read once more.
	metrics = vtesting.GetMetricsDifference(self.T(), metric_name, snapshot)
	client_queue_list_ops, _ = metrics.GetInt64(
		"datastore_latency__list_MemcacheDatastore_ClientTaskQueue_inf")
	assert.Equal(self.T(), int64(2), client_queue_list_ops)

	// Further reads are coming from the cache.
	for i := 0; i < 100; i++ {
		tasks, err := client_info_manager.GetClientTasks(self.client_id)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), 0, len(tasks))
	}

	// With no additional datastore access
	metrics = vtesting.GetMetricsDifference(self.T(), metric_name, snapshot)
	client_queue_list_ops, _ = metrics.GetInt64(
		"datastore_latency__list_MemcacheDatastore_ClientTaskQueue_inf")
	assert.Equal(self.T(), int64(2), client_queue_list_ops)
}
