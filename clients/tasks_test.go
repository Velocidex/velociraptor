package clients_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/clients"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
)

type ClientTasksTestSuite struct {
	test_utils.TestSuite

	client_id string
}

func (self *ClientTasksTestSuite) TestQueueMessages() {
	client_id := "C.1236"

	message1 := &crypto_proto.VeloMessage{Source: "Server", SessionId: "1"}
	err := clients.QueueMessageForClient(self.ConfigObj, client_id, message1)
	assert.NoError(self.T(), err)

	// Now retrieve all messages.
	tasks, err := clients.GetClientTasks(
		self.ConfigObj, client_id, true /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(tasks))
	assert.True(self.T(), proto.Equal(tasks[0], message1))

	// We did not lease, so the tasks are not removed, but this
	// time we will lease.
	tasks, err = clients.GetClientTasks(
		self.ConfigObj, client_id, false /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 1)

	// No tasks available.
	tasks, err = clients.GetClientTasks(
		self.ConfigObj, client_id, false /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 0)
}

func (self *ClientTasksTestSuite) TestFastQueueMessages() {
	client_id := "C.1235"

	written := []*crypto_proto.VeloMessage{}

	for i := 0; i < 10; i++ {
		message := &crypto_proto.VeloMessage{Source: "Server", SessionId: fmt.Sprintf("%d", i)}
		err := clients.QueueMessageForClient(self.ConfigObj, client_id, message)
		assert.NoError(self.T(), err)

		written = append(written, message)
	}

	// Now retrieve all messages.
	tasks, err := clients.GetClientTasks(
		self.ConfigObj, client_id, true /* do_not_lease */)
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

func TestClientTasksService(t *testing.T) {
	suite.Run(t, &ClientTasksTestSuite{})
}
