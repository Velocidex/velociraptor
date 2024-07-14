package http_comms_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

func (self *TestSuite) TestWebSocketRetry() {
	ws_connections_factory := http_comms.NewWebSocketConnectionFactoryForTests()
	http_comms.WSConnectorFactory = ws_connections_factory
	defer func() {
		http_comms.WSConnectorFactory = http_comms.WebSocketConnectionFactoryImpl{}
	}()

	self.client_url = fmt.Sprintf("ws://localhost:%d/", self.port)
	self.ConfigObj.Client.WsPingWaitSec = 1

	// Bring up the server
	self.ConfigObj.Frontend.BindPort = uint32(self.port)
	self.ConfigObj.Client.ServerUrls = []string{self.client_url}

	server_ctx, server_cancel := context.WithCancel(self.Ctx)
	defer server_cancel()

	server_wg := &sync.WaitGroup{}
	self.makeServer(server_ctx, server_wg)

	client_ctx, client_cancel := context.WithCancel(self.Ctx)
	defer client_cancel()

	client_wg := &sync.WaitGroup{}

	// Create a HTTP Client Communicator to play with
	self.makeClient(client_ctx, client_wg)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return strings.Contains(getMemoryLogs(), "reader: Received Ping")
	})

	for i := 0; i < 10; i++ {
		logging.ClearMemoryLogs()

		// Suddenly close the reader multiple times
		conn, pres := ws_connections_factory.GetConn("/reader")
		assert.True(self.T(), pres)

		fmt.Printf("Closing reader!\n")
		conn.Close()

		vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
			logs := getMemoryLogs()
			// We retry a few times,
			return matchMultipleRegex(logs,
				"Uninstalling connector .+ for ws://.+/reader",
				"Installing connector .+ to ws://.+/reader",
			) ||

				// But finally we give up and back off
				matchMultipleRegex(logs,
					"Exceeded retry times for reader",
					"Waiting for a reachable server:",
				)
		})

		if matchMultipleRegex(getMemoryLogs(), "Exceeded retry times for reader") {
			break
		}
	}

	// After exceeding the retry count, we back off anyway.
	assert.True(self.T(),
		matchMultipleRegex(getMemoryLogs(),
			"Exceeded retry times for reader",
			"Waiting for a reachable server:"))

	// Wait for reconnection
	vtesting.WaitUntil(10*time.Second, self.T(), func() bool {
		return strings.Contains(getMemoryLogs(), "Installing connector")
	})

	// Now take down the server.
	logging.ClearMemoryLogs()
	server_cancel()
	ws_connections_factory.Shutdown()

	// The client will try connecting 3 times and then give up and back off
	vtesting.WaitUntil(10*time.Second, self.T(), func() bool {
		logs := getMemoryLogs()
		return matchMultipleRegex(logs,
			"Retrying connection to reader for 0 time",
			"Retrying connection to reader for 1 time",
			"Retrying connection to reader for 2 time",
			"Exceeded retry times for reader",
			"Post to ws://.+/reader returned WS Socket is not connected - advancing to next server",
			"Waiting for a reachable server")
	})
}

func getMemoryLogs() (res string) {
	for _, l := range logging.GetMemoryLogs() {
		res += l
	}
	return res
}

func matchMultipleRegex(in string, regex ...string) bool {
	for _, re := range regex {
		compiled := regexp.MustCompile(re)
		if !compiled.MatchString(in) {
			return false
		}
	}

	return true
}
