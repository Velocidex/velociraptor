/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package http_comms

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_test "www.velocidex.com/golang/velociraptor/crypto/testing"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/rand"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	counter utils.Counter
)

type EventRecorder struct {
	mu     sync.Mutex
	events []string
}

func (self *EventRecorder) Add(msg string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.events = append(self.events, msg)
}

func (self *EventRecorder) Len() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return len(self.events)
}

func (self *EventRecorder) Get() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return append([]string{}, self.events...)
}

type FakeClock struct {
	*utils.MockClock

	mu     sync.Mutex
	events *EventRecorder
}

func (self *FakeClock) After(d time.Duration) <-chan time.Time {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.events.Add(fmt.Sprintf("%d sleep: %v\n", counter.Get(), d))
	counter.Inc()
	return time.After(0)
}

func (self *FakeClock) Sleep(d time.Duration) {
	<-self.After(d)
}

type Response struct {
	data     string
	status   int
	location string
}

type FakeServer struct {
	name string

	events *EventRecorder

	resp_idx  int
	responses []*Response

	server *httptest.Server
	URL    string
}

func (self *FakeServer) Close() {
	self.server.Close()
}

func (self *FakeServer) Log(fmtstring string, args ...interface{}) {
	msg := fmt.Sprintf("%d ", counter.Get())
	counter.Inc()
	msg += fmt.Sprintf(fmtstring, args...)
	self.events.Add(msg)
}

func NewFakeServer(name string) *FakeServer {
	self := &FakeServer{
		name:   name,
		events: &EventRecorder{},
	}
	self.server = httptest.NewServer(api_utils.HandlerFunc(nil,
		func(rw http.ResponseWriter, req *http.Request) {
			self.Log("request: %v", req.URL.String())

			// Break out of a runaway condition.
			if self.events.Len() > 100 {
				fmt.Printf("Something went horribly wrong")
				utils.Debug(self.events.Get())
				os.Exit(-1)
			}

			encrypted := &bytes.Buffer{}

			io.Copy(encrypted, req.Body)
			if self.resp_idx >= len(self.responses) {
				http.Error(rw, "OK", 200)
				self.Log("response: default OK 200")
				self.resp_idx = 0
				return
			}

			response := self.responses[self.resp_idx]
			if response.location != "" {
				rw.Header()["Location"] = []string{response.location}
			}

			if response.status == 200 {
				self.Log("response: %v 200", response.data)
				rw.Write([]byte(response.data))
			} else {
				self.Log("response: %v %d", response.data, response.status)
				http.Error(rw, "OK", response.status)
			}

			self.resp_idx++
		}))
	self.URL = self.server.URL + "/"
	return self
}

type CommsTestSuite struct {
	suite.Suite

	frontend1, frontend2 *FakeServer
	config_obj           *config_proto.Config

	empty_response []byte
	closer         func()

	writeback_path string
}

func (self *CommsTestSuite) SetupTest() {
	counter = utils.Counter{}

	self.frontend1 = NewFakeServer("frontend1")
	self.frontend2 = NewFakeServer("frontend2")

	config_obj, err := new(config.Loader).WithFileLoader(
		"../http_comms/test_data/server.config.yaml").
		WithRequiredClient().WithWriteback().LoadAndValidate()

	require.NoError(self.T(), err)
	require.NoError(self.T(), config.ValidateClientConfig(config_obj))

	self.config_obj = config_obj
	self.config_obj.Client.LocalBuffer.DiskSize = 0

	cm := &crypto_test.NullCryptoManager{}
	self.empty_response, _ = cm.EncryptMessageList(
		&crypto_proto.MessageList{},
		self.config_obj.Client.Nonce,
		"C.1234")

	// Disable randomness for the test.
	self.closer = rand.DisableRand()

	// Initialize the writeback
	self.writeback_path = getTempFile(self.T())
	self.config_obj.Client.WritebackLinux = self.writeback_path
	self.config_obj.Client.WritebackWindows = self.writeback_path
	self.config_obj.Client.WritebackDarwin = self.writeback_path

	writeback_service := writeback.GetWritebackService()
	writeback_service.LoadWriteback(self.config_obj)
}

func (self *CommsTestSuite) TearDownTest() {
	self.frontend1.Close()
	self.frontend2.Close()
	self.closer()

	os.Remove(self.writeback_path)
}

// Check that unexpected closing of the executor calls the abort
// function.
func (self *CommsTestSuite) TestAbort() {
	var mu sync.Mutex

	func_called := false
	on_error := func() {
		mu.Lock()
		func_called = true
		mu.Unlock()
	}

	urls := []string{self.frontend1.URL}

	// Not a real executor but we can emulate closing channels.
	exec := executor.NewClientExecutorForTests(self.config_obj)

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		exec, urls, on_error, utils.RealClock{})
	assert.NoError(self.T(), err)

	// Start a communicator feeding data to the executor.
	wg.Add(1)
	go func() {
		defer wg.Done()

		communicator.Run(ctx, wg)
	}()

	// Emulate the case of the executor exiting early - this
	// should never happen in practice but might happen due to a
	// bug or panic(). If this is allowed to go unchecked we might
	// get into a state where the client does not have a working
	// executor and can not be reached. The only sensible thing to
	// do in this case is to abort which happens via the provided
	// on_error callback.
	close(exec.Outbound)

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		return func_called
	})

	assert.Equal(self.T(), func_called, true)
}

func (self *CommsTestSuite) TestEnrollment() {
	urls := []string{self.frontend1.URL}
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	self.frontend1.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: "", status: 406},
		{data: "", status: 406},
		{data: "", status: 406},
	}

	communicator.receiver.sendMessageList(
		context.Background(), nil, !URGENT,
		crypto_proto.PackedMessageList_ZCOMPRESSION)

	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem but fails on frontend1
		"0 request: /server.pem",
		"1 response: -----BEG",
		"2 request: /reader",

		// A 406 should not trigger resend - we just schedule
		// a new CSR to go in the next poll.
		"3 response:  406",
	})
}

func (self *CommsTestSuite) TestServerError() {
	urls := []string{self.frontend1.URL}
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	self.frontend1.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: "", status: 500},
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	communicator.receiver.sendMessageList(
		context.Background(), nil, !URGENT,
		crypto_proto.PackedMessageList_ZCOMPRESSION)

	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem
		"request: /server.pem",
		"response: -----BEGIN CERTIFICATE-----",

		// We will fail the next request with a 500
		"request: /reader",
		"response:  500",

		// Client will sleep to back off and try to rekey
		"sleep: 10m0s\n",
		"sleep: 10m0s\n",
		"request: /server.pem",
		"response: -----BEGIN CERTIFICATE-----",

		// This one worked and should be successful.
		"request: /reader",
		"response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",
	})
}

// Client configured with two frontends. Frontend1 is down returning
// 500, Frontend2 is down too.
func (self *CommsTestSuite) TestMultiFrontends() {
	urls := []string{self.frontend1.URL, self.frontend2.URL}
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(
		ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	// Frontend1 will return 500 all the time.
	self.frontend1.responses = []*Response{
		{data: "", status: 500},
		{data: "", status: 500},
		{data: "", status: 500},
	}

	// Frontend2 errors at first but then comes back online
	self.frontend2.responses = []*Response{
		{data: "", status: 500},
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	assert.Equal(self.T(), urls[0], self.frontend1.URL)

	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)

	// Message ordering is important
	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem but fails on frontend1
		"0 request: /server.pem",
		"1 response:  500",

		// After trying frontend2 immediately, we back off all frontends
		"4 sleep: 10m0s\n",

		// We try frontend1 again but again it fails
		"5 request: /server.pem",
		"6 response:  500",
	})

	checkResponses(self.T(), self.frontend2.events.Get(), []string{
		// First request on FE 2 fails.
		"2 request: /server.pem",
		"3 response:  500",

		// Second round - try FE 2 again - works this time.
		"7 request: /server.pem",
		"8 response: -----BEGIN CERTIFICATE-----",

		// This one worked and should be successful.
		"9 request: /reader",
		"10 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",
	})
}

// Client configured with two frontends. Both keep failing.
func (self *CommsTestSuite) TestMultiFrontendsAllIsBorked() {
	urls := []string{self.frontend1.URL, self.frontend2.URL}
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	// Frontend1 will return 500 all the time.
	self.frontend1.responses = []*Response{
		{data: "", status: 500},
		{data: "", status: 500},
		{data: "", status: 500},
	}

	// Frontend2 errors at first but then comes back online
	self.frontend2.responses = []*Response{
		{data: "", status: 500},
		{data: "", status: 500},
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)

	//utils.Debug(self.frontend1.events.Get())
	//utils.Debug(self.frontend2.events.Get())

	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem but fails on frontend1
		"0 request: /server.pem",
		"1 response:  500",

		// Must sleep now as both FE are down.
		"4 sleep: 10m0s\n",

		// Try FE1 again
		"5 request: /server.pem",
		"6 response:  500",

		// Must sleep again
		"9 sleep: 10m0s\n",
		"10 request: /server.pem",
		"11 response:  500",
	})

	checkResponses(self.T(), self.frontend2.events.Get(), []string{
		// Immediately switch to FE2 but it is down as well.
		"2 request: /server.pem",
		"3 response:  500",

		// Immediately switch to FE2 - no sleep
		"7 request: /server.pem",
		"8 response:  500",

		"12 request: /server.pem",
		"13 response: -----BEGIN CERTIFICATE-----\n",

		// This one worked and should be successful.
		"14 request: /reader",
		"15 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",
	})
}

// With 2 FE configured if FE 1 fails intermittantly (perhaps due to
// load), client should back off and try FE1 one more time before
// switching to FE2
func (self *CommsTestSuite) TestMultiFrontendsIntermittantFailure() {
	urls := []string{self.frontend1.URL, self.frontend2.URL}
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	// FE1 is not completely off just loaded - so initial
	// server.pem would work but processing is not possible.
	self.frontend1.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},

		// Emit a single failure.
		{data: "", status: 500},
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	self.frontend2.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)

	//utils.Debug(self.frontend1.events.Get())
	//utils.Debug(self.frontend2.events.Get())

	// Message ordering is important
	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem is ok.
		"0 request: /server.pem",
		"1 response: -----BEGIN CERTIFICATE-",

		// Now client tries to connect for real.
		"2 request: /reader",
		"3 response:  500",
		"4 sleep: 10m0s\n",
	})

	checkResponses(self.T(), self.frontend2.events.Get(), []string{
		"5 request: /server.pem",
		"6 response: -----BEGIN CERTIFICAT",

		// This time we get through.
		"7 request: /reader",
		"8 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",
	})
}

// With 2 FE configured if FE 1 fails we switch to FE2 and when that
// fails we switch back to FE1.
func (self *CommsTestSuite) TestMultiFrontendsHeavyFailure() {
	urls := []string{self.frontend1.URL, self.frontend2.URL}
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	// FE1 is not completely off just loaded - so initial
	// server.pem would work but processing is not possible.
	self.frontend1.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},

		// Emit a single failure.
		{data: "", status: 500},

		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	self.frontend2.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: "", status: 500},
	}

	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)

	//utils.Debug(self.frontend1.events.Get())
	//utils.Debug(self.frontend2.events.Get())

	// Message ordering is important
	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem is ok.
		"0 request: /server.pem",
		"1 response: -----BEGIN CERTIFICATE-",

		// Now client tries to connect for real - failed and
		// switch immediately to FE2
		"2 request: /reader",
		"3 response:  500",

		"4 sleep: 10m0s\n",
		"9 sleep: 10m0s\n",
		"10 sleep: 10m0s\n",

		// Sleep after switching back from FE2
		"11 request: /server.pem",
		"12 response: -----BEGIN CERTIFICATE-",
		"13 request: /reader",
		"14 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",
	})

	checkResponses(self.T(), self.frontend2.events.Get(), []string{
		// Rekey FE2
		"5 request: /server.pem",

		// Still failing will now switch to FE2
		"6 response: -----BEGIN CERTIFICAT",

		// ERROR - switch back but this time we sleep.
		"7 request: /reader",
		"8 response:  500",
	})
}

// With 2 FE configured. FE1 redirects to FE2
func (self *CommsTestSuite) TestMultiFrontendRedirect() {
	// FE2 is not known to the client in advance.
	urls := []string{self.frontend1.URL}
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	// FE1 is not completely off just loaded - so initial
	// server.pem would work but processing is not possible.
	self.frontend1.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},

		// Send the client to FE2
		{data: "", status: 301, location: self.frontend2.URL},
	}

	self.frontend2.responses = []*Response{
		// Client will rekey to FE2 and this request will work.
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
		{data: string(self.empty_response), status: 200},
	}

	// Request 2 packets.
	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)
	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)

	//utils.Debug(self.frontend1.events.Get())
	//utils.Debug(self.frontend2.events.Get())

	// Message ordering is important
	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem is ok.
		"0 request: /server.pem",
		"1 response: -----BEGIN CERTIFICATE-",

		// Now client tries to connect for real.
		"2 request: /reader",
		"3 response:  301",
	})

	checkResponses(self.T(), self.frontend2.events.Get(), []string{
		// Rekey this FE
		"4 request: /server.pem",
		"5 response: -----BEGIN CERTIFIC",

		// This FE is up.
		"6 request: /reader?r=1",
		"7 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",

		// Next request goes straight to this FE and includes
		// the r=1 parameter to avoid another redirect.
		"8 request: /reader?r=1",
		"9 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",
	})
}

// Check that redirects do not cause un-neccesary sleeps.
func (self *CommsTestSuite) TestMultiFrontendRedirectWithErrors() {
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	urls := []string{self.frontend1.URL}

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	self.frontend1.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},

		// Send the client to FE2
		{data: "", status: 301, location: self.frontend2.URL},

		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: "", status: 500},

		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	self.frontend2.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},

		{data: "", status: 500},
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)
	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)

	//utils.Debug(self.frontend1.events.Get())
	//utils.Debug(self.frontend2.events.Get())

	// Message ordering is important
	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem is ok.
		"0 request: /server.pem",
		"1 response: -----BEGIN CERTIFICATE-",

		// FE1 -> FE2 redirection
		"2 request: /reader",
		"3 response:  301",

		// Immediately switch to FE1 (no sleep)
		"10 sleep: 10m0s\n",
		"11 request: /server.pem",
		"12 response: -----BEGIN CERTIFICATE-",

		// Try to connect to FE1 but there is an error. NOTE
		// r=1 is now removed.
		"13 request: /reader",
		"14 response:  500",

		// Now must sleep since we tried all endpoints and
		// they all failed.
		"15 sleep: 10m0s\n",
		"16 sleep: 10m0s\n",
	})

	checkResponses(self.T(), self.frontend2.events.Get(), []string{
		// Rekey FE2
		"4 request: /server.pem",
		"5 response: -----BEGIN CERTIFIC",

		// This FE is up.
		"6 request: /reader?r=1",
		"7 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",

		// Next request goes straight to this FE and includes
		// the r=1 parameter to avoid another redirect. FE2 is
		// down now.
		"8 request: /reader?r=1",
		"9 response:  500",

		// After sleep switch to FE2 and succeed.
		"17 request: /server.pem",
		"18 response: -----BEGIN CERTIFIC",
		"19 request: /reader",
		"20 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",
	})
}

// Frontends redirecting to each other.
func (self *CommsTestSuite) TestMultiRedirects() {
	urls := []string{self.frontend1.URL}
	mock_clock := utils.NewMockClock(time.Unix(100, 0))
	cancel := utils.MockTime(mock_clock)
	defer cancel()

	clock := &FakeClock{
		MockClock: mock_clock,
		events:    self.frontend1.events,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crypto_manager := &crypto_test.NullCryptoManager{}
	communicator, err := NewHTTPCommunicator(ctx, self.config_obj, crypto_manager,
		executor.NewTestExecutor(), urls, nil, clock)
	assert.NoError(self.T(), err)

	self.frontend1.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},
		// Send the client to FE2
		{data: "", status: 301, location: self.frontend2.URL},

		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	self.frontend2.responses = []*Response{
		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: "", status: 301, location: self.frontend1.URL},

		{data: self.config_obj.Frontend.Certificate, status: 200},
		{data: string(self.empty_response), status: 200},
	}

	communicator.receiver.sendMessageList(context.Background(), nil,
		!URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)

	//utils.Debug(self.frontend1.events.Get())
	//utils.Debug(self.frontend2.events.Get())

	// Multiple redirections should not add duplicates to the url list.
	assert.Equal(self.T(), communicator.receiver.connector.(*HTTPConnector).urls,
		[]string{self.frontend1.URL, self.frontend2.URL})

	// Message ordering is important
	checkResponses(self.T(), self.frontend1.events.Get(), []string{
		// First request looks for server.pem is ok.
		"0 request: /server.pem",
		"1 response: -----BEGIN CERTIFICATE-",

		// FE1 -> FE2 redirection
		"2 request: /reader",
		"3 response:  301",

		"8 sleep: 10m0s\n",

		// Immediately switch to FE1 (no sleep)
		"9 request: /server.pem",
		"10 response: -----BEGIN CERTIFICATE-",

		"11 request: /reader?r=1",
		"12 response: \n\vx\x01\x01\x00\x00\xff\xff\x00\x00\x00\x01 200",
	})

	checkResponses(self.T(), self.frontend2.events.Get(), []string{
		// Rekey FE2
		"4 request: /server.pem",
		"5 response: -----BEGIN CERTIFIC",

		// This FE is up.
		"6 request: /reader?r=1",
		"7 response:  301",
	})
}

func checkResponses(t *testing.T, seen []string, expected []string) {
	for idx, seen_line := range seen {
		assert.Contains(t, seen_line, expected[idx])
	}

	assert.Equal(t, len(expected), len(seen))
}

func TestHTTPComms(t *testing.T) {
	suite.Run(t, new(CommsTestSuite))
}
