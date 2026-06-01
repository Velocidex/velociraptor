package http_comms_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/http_comms/e2e"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"

	// For clock()
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/common"
)

type TestSuite struct {
	e2e.E2ETestSuite
}

// Start a collection, and see if it works.
func (self *TestSuite) TestScheduleCollection() {
	closer := self.testScheduleCollection()
	defer closer()
}

func (self *TestSuite) TestScheduleCollectionWithWebSocket() {
	self.ClientUrl = fmt.Sprintf("ws://localhost:%d/", self.Port)
	self.ConfigObj.Client.WsPingWaitSec = 1

	closer := self.testScheduleCollection()
	defer closer()

	time.Sleep(3 * time.Second)

	// Make sure the server sent some pings to the client
	pings := []string{}
	for _, l := range logging.GetMemoryLogs() {
		if strings.Contains(l, "Ping") {
			pings = append(pings, l)
		}
	}
	assert.True(self.T(), len(pings) > 5)
}

func (self *TestSuite) testScheduleCollection() (closer func()) {
	closer = self.StartServerAndClient()

	// Schedule a collection on the client
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.ClientId,
		Artifacts: []string{"TestArtifact"},
		Creator:   utils.GetSuperuserName(self.ConfigObj),
	}

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)
	assert.NoError(self.T(), err)

	var flow *api_proto.FlowDetails

	vtesting.WaitUntil(20*time.Second, self.T(), func() bool {
		flow, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.ClientId, flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.State == flows_proto.ArtifactCollectorContext_FINISHED
	})
	assert.Equal(self.T(), uint64(5), flow.Context.TotalLogs)
	assert.Equal(self.T(), uint64(1), flow.Context.TotalCollectedRows)
	assert.Equal(self.T(), "TestArtifact", flow.Context.ArtifactsWithResults[0])

	return closer
}

func (self *TestSuite) TestServerRotateKeyE2E() {
	logging.ClearMemoryLogs()

	self.ConfigObj.Frontend.BindPort = uint32(self.Port)
	self.ConfigObj.Client.ServerUrls = []string{
		fmt.Sprintf("http://localhost:%d", self.Port),
	}

	server_ctx, server_cancel := context.WithCancel(self.Ctx)
	server_wg := &sync.WaitGroup{}

	self.MakeServer(server_ctx, server_wg)

	client_ctx, client_cancel := context.WithCancel(self.Ctx)
	client_wg := &sync.WaitGroup{}

	comm := self.MakeClient(client_ctx, client_wg)

	// Make sure the client is properly enrolled
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		err := comm.Sender.SendToURL(client_ctx, [][]byte{},
			!http_comms.URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)
		assert.NoError(self.T(), err)

		return vtesting.ContainsString("response with status: 200",
			logging.GetMemoryLogs())
	})

	// Stop the receive and send loops to prevent race with direct post
	comm.SetPause(true)

	logging.ClearMemoryLogs()

	// Now rotate the server keys: First shut down the old server.
	server_cancel()
	server_wg.Wait()

	// Now rekey the server
	frontend_cert, err := crypto.GenerateServerCert(
		self.ConfigObj, utils.GetSuperuserName(self.ConfigObj))
	assert.NoError(self.T(), err)

	self.ConfigObj.Frontend.Certificate = frontend_cert.Cert
	self.ConfigObj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Now bring up the new server.
	server_ctx, server_cancel = context.WithCancel(self.Ctx)
	server_wg = &sync.WaitGroup{}

	logging.ClearMemoryLogs()

	self.MakeServer(server_ctx, server_wg)

	// Make sure the client properly rekeys and continues to talk to the server
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		err := comm.Sender.SendToURL(client_ctx, [][]byte{},
			!http_comms.URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)
		if err != nil {
			return false
		}

		// Make sure the client rekeys and connects successfully.
		return vtesting.ContainsString("response with status: 200",
			logging.GetMemoryLogs()) &&
			vtesting.ContainsString("Received PEM for VelociraptorServer",
				logging.GetMemoryLogs())
	})

	// Done
	server_cancel()
	client_cancel()

	// Wait for the server to quit.
	client_wg.Wait()
	server_wg.Wait()
}

func TestClientServerComms(t *testing.T) {
	res := &TestSuite{}
	res.Artifacts = []string{`
name: TestArtifact
sources:
- query: SELECT log(message="Hello") AS Hello FROM scope()
`}
	suite.Run(t, res)
}
