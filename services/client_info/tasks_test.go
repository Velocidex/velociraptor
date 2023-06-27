package client_info_test

import (
	"context"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *ClientInfoTestSuite) TestQueueMessages() {
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	message1 := &crypto_proto.VeloMessage{Source: "Server", SessionId: "1"}
	err = client_info_manager.QueueMessageForClient(
		context.Background(),
		self.client_id, message1,
		services.NOTIFY_CLIENT, utils.BackgroundWriter)
	assert.NoError(self.T(), err)

	manager := client_info_manager.(*client_info.ClientInfoManager)

	// Wait here until the tasks is visible.
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		tasks, err := manager.PeekClientTasks(context.Background(), self.client_id)
		assert.NoError(self.T(), err)
		return len(tasks) == 1 && proto.Equal(tasks[0], message1)
	})

	// We did not lease, so the tasks are not removed, but this
	// time we will lease.
	tasks, err := manager.GetClientTasks(context.Background(), self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 1)

	// No tasks available.
	tasks, err = manager.PeekClientTasks(context.Background(), self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 0)
}

func (self *ClientInfoTestSuite) TestFastQueueMessages() {
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	written := []*crypto_proto.VeloMessage{}

	for i := 0; i < 10; i++ {
		message := &crypto_proto.VeloMessage{Source: "Server", SessionId: fmt.Sprintf("%d", i)}
		err := client_info_manager.QueueMessageForClient(
			context.Background(),
			self.client_id, message,
			services.NOTIFY_CLIENT, utils.BackgroundWriter)
		assert.NoError(self.T(), err)

		written = append(written, message)
	}

	// Wait until all messages are visible
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		tasks, err := client_info_manager.PeekClientTasks(
			context.Background(), self.client_id)
		assert.NoError(self.T(), err)
		return 10 == len(tasks)
	})

	tasks, err := client_info_manager.GetClientTasks(
		context.Background(), self.client_id)
	assert.NoError(self.T(), err)

	// Does not have to return in sorted form.
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].SessionId < tasks[j].SessionId
	})

	for i := 0; i < 10; i++ {
		assert.True(self.T(), proto.Equal(tasks[i], written[i]))
	}
}
