package logscale

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"net"
	"net/http"
	"net/http/httptest"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"

	"github.com/Velocidex/ordereddict"

	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	validUrl              = "https://cloud.community.humio.com/api"
	validAuthToken        = "valid-ingest-token"
	validWorkerCount      = 1
	invalidWorkerCount    = -11
	testTimestampStringTZ = "2023-04-05T13:36:51-04:00"
	testTimestampUNIX     = uint64(1680716211) // json ints are uint64
	testTimestamp         = "2023-04-05T17:36:51Z"
	testClientId          = "C.0030300330303000"
	testHostname          = "testhost12"

	gMaxPollDev = 30
)

type LogScaleQueueTestSuite struct {
	test_utils.TestSuite

	queue *LogScaleQueue
	scope vfilter.Scope
	ctx   context.Context

	repoManager services.RepositoryManager
	timestamp   time.Time
	clients     []string

	server *httptest.Server
	start  time.Time

	restoreClock    func()
	wantConnRefused bool
}

func formatTimestamp(ts time.Time) string {
	// json.MarshalWithOptions(payloads, opts) will turn this into UTC
	return ts.UTC().Format(time.RFC3339Nano)
}

func (self *LogScaleQueueTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.TestSuite.SetupTest()
	self.start = time.Now()

	self.queue = NewLogScaleQueue(self.ConfigObj)
	self.queue.SetHttpClientTimeoutDuration(time.Duration(1) * time.Second)
	self.queue.SetMaxRetries(1)
	self.scope = self.getScope()

	self.restoreClock = utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))

	self.ctx = context.Background()
	self.populateClients()
	self.queue.SetHttpTransport(self.getHttpTransport())
	self.wantConnRefused = false
}

func (self *LogScaleQueueTestSuite) populateClients() {
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_info_manager.Set(self.Ctx,
		&services.ClientInfo{
			ClientInfo: &actions_proto.ClientInfo{
				ClientId: "C.0030300330303000",
				Hostname: "testhost12",
			}})
}

func (self *LogScaleQueueTestSuite) getHttpTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	realDialContext := transport.DialContext

	dc := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if self.wantConnRefused {
			return nil, syscall.ECONNREFUSED
		} else {
			return realDialContext(ctx, network, addr)
		}
	}

	transport.DialContext = dc
	return transport
}

func (self *LogScaleQueueTestSuite) getScope() vfilter.Scope {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	require.NoError(self.T(), err)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(self.ConfigObj,
			&logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	return manager.BuildScope(builder)
}

func generateRow() *ordereddict.Dict {
	return ordereddict.NewDict().
		Set("_ts", testTimestampUNIX).
		Set("TestValue1", "value1").
		Set("TestValue2", "value2").
		Set("TestValue3", "value3").
		Set("TestValue4", "value4").
		Set("Artifact", "LogScale.Client.Events").
		Set("ClientId", testClientId)
}

func (self *LogScaleQueueTestSuite) TearDownTest() {
	if self.queue != nil {
		self.queue.Close(self.scope)
	}
	self.restoreClock()
}

func (self *LogScaleQueueTestSuite) TestEmptyUrl() {
	err := self.queue.Open(self.ctx, self.scope, "", validAuthToken)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestInvalidUrl() {
	err := self.queue.Open(self.ctx, self.scope, "invalid-url", validAuthToken)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestValidUrl() {
	err := self.queue.Open(self.ctx, self.scope, validUrl, validAuthToken)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestEmptyAuthToken() {
	err := self.queue.Open(self.ctx, self.scope, validUrl, "")
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestValidAuthToken() {
	err := self.queue.Open(self.ctx, self.scope, validUrl, validAuthToken)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestInvalidThreads() {
	err := self.queue.SetWorkerCount(-1)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestValidThreads() {
	err := self.queue.SetWorkerCount(validWorkerCount)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestSetEventBatchSizeValid() {
	err := self.queue.SetEventBatchSize(10)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestSetEventBatchSizeZero() {
	err := self.queue.SetEventBatchSize(0)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestSetEventBatchSizeNegative() {
	err := self.queue.SetEventBatchSize(-10)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestSetBatchingTimeoutDurationValid() {
	err := self.queue.SetBatchingTimeoutDuration(10 * time.Second)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestSetBatchingTimeoutDurationZero() {
	err := self.queue.SetBatchingTimeoutDuration(0 * time.Second)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestSetBatchingTimeoutDurationNegative() {
	err := self.queue.SetBatchingTimeoutDuration(-10 * time.Second)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestSetHttpClientTimeoutDurationValid() {
	err := self.queue.SetHttpClientTimeoutDuration(10 * time.Second)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestSetHttpClientTimeoutDurationZero() {
	err := self.queue.SetHttpClientTimeoutDuration(0 * time.Second)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestSetHttpClientTimeoutDurationNegative() {
	err := self.queue.SetHttpClientTimeoutDuration(-10 * time.Second)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestSetHttpTransportNil() {

	self.queue.SetHttpTransport(nil)

	// Special case: We want to do the processing ourselves
	self.queue.nWorkers = 0

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)
	require.NotNil(self.T(), self.queue.httpClient.Transport)
}

func (self *LogScaleQueueTestSuite) TestSetHttpTransportValid() {

	transport := http.DefaultTransport.(*http.Transport).Clone()

	self.queue.SetHttpTransport(transport)

	// Special case: We want to do the processing ourselves
	self.queue.nWorkers = 0

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)
	require.NotNil(self.T(), self.queue.httpClient.Transport)
}

func (self *LogScaleQueueTestSuite) TestMaxRetriesZero() {
	err := self.queue.SetMaxRetries(0)
	require.NoError(self.T(), err)
	require.Equal(self.T(), self.queue.maxRetries, 0)
}

func (self *LogScaleQueueTestSuite) TestMaxRetriesNegative() {
	err := self.queue.SetMaxRetries(-100)
	require.NoError(self.T(), err)
	require.Equal(self.T(), self.queue.maxRetries, -100)
}

func (self *LogScaleQueueTestSuite) TestMaxRetriesPositive() {
	err := self.queue.SetMaxRetries(100)
	require.NoError(self.T(), err)
	require.Equal(self.T(), self.queue.maxRetries, 100)
}

func (self *LogScaleQueueTestSuite) TestSetTaggedFieldsValid() {
	args := []string{"x=y", "y=z", "z"}
	expected := map[string]string{
		"x": "y",
		"y": "z",
		"z": "z",
	}
	err := self.queue.SetTaggedFields(args)
	require.NoError(self.T(), err)
	require.EqualValues(self.T(), self.queue.tagMap, expected)
}

func (self *LogScaleQueueTestSuite) TestSetTaggedFieldsMultipleEquals() {
	args := []string{"x=y", "y=z=z"}
	expected := map[string]string{
		"x": "y",
		"y": "z=z",
	}
	err := self.queue.SetTaggedFields(args)
	require.NoError(self.T(), err)
	require.EqualValues(self.T(), self.queue.tagMap, expected)
}

func (self *LogScaleQueueTestSuite) TestSetTaggedFieldsEmptyTagName() {
	args := []string{"=y", "y=z", "z"}
	err := self.queue.SetTaggedFields(args)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) TestSetTaggedFieldsEmptyTagArg() {
	args := []string{}
	err := self.queue.SetTaggedFields(args)
	require.NoError(self.T(), err)
	require.Nil(self.T(), self.queue.tagMap)
}

func (self *LogScaleQueueTestSuite) TestSetTaggedFieldsEmptyTagString() {
	args := []string{""}
	err := self.queue.SetTaggedFields(args)
	require.NotNil(self.T(), err)
	require.ErrorAs(self.T(), err, &errInvalidArgument{})
}

func (self *LogScaleQueueTestSuite) checkTimestamp(payload *LogScalePayload) {
	require.Equal(self.T(), testTimestamp, formatTimestamp(payload.Events[0].Timestamp))
	require.Equal(self.T(), "", payload.Events[0].Timezone)
}

func (self *LogScaleQueueTestSuite) TestTimestamp_TimeString() {
	row := ordereddict.NewDict().
		Set("Time", testTimestampStringTZ).
		Set("timestamp", testTimestampStringTZ).
		Set("_ts", testTimestampStringTZ).
		Set("TestValue", "value")

	payload := NewLogScalePayload(row)

	self.queue.addTimestamp(self.ctx, self.scope, row, payload)

	self.checkTimestamp(payload)
}

func (self *LogScaleQueueTestSuite) TestTimestamp_timestampString() {
	row := ordereddict.NewDict().
		Set("timestamp", testTimestampStringTZ).
		Set("_ts", testTimestampStringTZ).
		Set("TestValue", "value")

	payload := NewLogScalePayload(row)

	self.queue.addTimestamp(self.ctx, self.scope, row, payload)

	self.checkTimestamp(payload)
}

func (self *LogScaleQueueTestSuite) TestTimestamp__tsString() {
	row := ordereddict.NewDict().
		Set("_ts", testTimestampStringTZ).
		Set("TestValue", "value")

	payload := NewLogScalePayload(row)

	self.queue.addTimestamp(self.ctx, self.scope, row, payload)

	self.checkTimestamp(payload)
}

func (self *LogScaleQueueTestSuite) TestTimestamp_TimeUNIX() {
	row := ordereddict.NewDict().
		Set("Time", testTimestampUNIX).
		Set("timestamp", testTimestampUNIX).
		Set("_ts", testTimestampUNIX).
		Set("TestValue", "value")

	payload := NewLogScalePayload(row)

	self.queue.addTimestamp(self.ctx, self.scope, row, payload)

	self.checkTimestamp(payload)
}

func (self *LogScaleQueueTestSuite) TestTimestamp_timestampUNIX() {
	row := ordereddict.NewDict().
		Set("timestamp", testTimestampUNIX).
		Set("_ts", testTimestampUNIX).
		Set("TestValue", "value")

	payload := NewLogScalePayload(row)

	self.queue.addTimestamp(self.ctx, self.scope, row, payload)

	self.checkTimestamp(payload)
}

func (self *LogScaleQueueTestSuite) TestTimestamp__tsUNIX() {
	row := ordereddict.NewDict().
		Set("_ts", testTimestampUNIX).
		Set("TestValue", "value")

	payload := NewLogScalePayload(row)

	self.queue.addTimestamp(self.ctx, self.scope, row, payload)

	self.checkTimestamp(payload)
}

func (self *LogScaleQueueTestSuite) TestAddMappedTags() {
	row := generateRow()

	expected := map[string]string{
		"TestValue3": "value3",
		"TestValue4": "value4",
	}

	expectedTags := []string{}
	for k := range expected {
		expectedTags = append(expectedTags, k)
	}

	payload := NewLogScalePayload(row)

	self.queue.SetTaggedFields(expectedTags)
	self.queue.addMappedTags(row, payload)

	actualTags := []string{}
	for k := range payload.Tags {
		actualTags = append(actualTags, k)
	}

	require.ElementsMatch(self.T(), expectedTags, actualTags)
	for k := range expected {
		require.Equal(self.T(), expected[k], payload.Tags[k])
	}
}

func (self *LogScaleQueueTestSuite) TestAddClientInfo() {
	row := generateRow()

	payload := NewLogScalePayload(row)
	self.queue.addClientInfo(self.ctx, row, payload)

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		return strings.Contains(fmt.Sprintf("%v", payload.Tags), "ClientHostname")
	})

	require.EqualValues(self.T(), payload.Tags["ClientHostname"], testHostname)
}

func (self *LogScaleQueueTestSuite) TestRowToPayload() {
	row := generateRow()

	expectedTags := []string{
		"TestValue3",
		"TestValue4",
		"ClientId",
		"ClientHostname",
	}

	self.queue.SetTaggedFields(expectedTags)

	payload := self.queue.rowToPayload(self.ctx, self.scope, row)

	expectedAttributes := row.Keys()
	actualAttributes := payload.Events[0].Attributes.Keys()

	require.EqualValues(self.T(), expectedAttributes, actualAttributes)
	for _, k := range expectedAttributes {
		expected, ok := row.Get(k)
		require.True(self.T(), ok)

		actual, ok := row.Get(k)
		require.True(self.T(), ok)

		require.EqualValues(self.T(), expected, actual)
	}

	actualTags := []string{}
	for k := range payload.Tags {
		actualTags = append(actualTags, k)
	}

	require.ElementsMatch(self.T(), expectedTags, actualTags)
	for _, k := range expectedTags {
		var val interface{}
		if k == "ClientHostname" {
			val = testHostname
		} else {
			var ok bool
			val, ok = row.Get(k)
			require.True(self.T(), ok)
		}
		require.EqualValues(self.T(), val, payload.Tags[k])
	}

	self.checkTimestamp(payload)
}

func (self *LogScaleQueueTestSuite) handleEndpointRequest(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if len(auth) < 8 || strings.ToLower(strings.TrimSpace(auth))[0:7] != "bearer " {
		w.WriteHeader(http.StatusUnauthorized)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		fmt.Fprintf(w, "The supplied authentication is invalid")
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		fmt.Fprintf(w, "Internal failure. reason=%s", err)
		return
	}

	data := []LogScalePayload{}

	err = json.Unmarshal(body, &data)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		fmt.Fprintf(w, "Could not handle input. reason=%s", err)
		return
	}

	// Empty submission is valid
	if len(data) > 0 {
		if len(data[0].Events) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
			fmt.Fprintf(w, "Could not handle input. reason=%s", "Could not parse JSON")
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{}")
}

func handler401(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusUnauthorized)
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	fmt.Fprintf(w, "unauthorized")
}

func handler500(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	fmt.Fprintf(w, "internal server error")
}

func (self *LogScaleQueueTestSuite) startMockServerWithHandler(handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimSpace(r.URL.Path) {
		case apiEndpoint:
			handler(w, r)
		default:
			http.NotFoundHandler().ServeHTTP(w, r)
		}
	}))
}

func (self *LogScaleQueueTestSuite) startMockServer() *httptest.Server {
	return self.startMockServerWithHandler(func(w http.ResponseWriter, r *http.Request) {
		self.handleEndpointRequest(w, r)
	})
}

func (self *LogScaleQueueTestSuite) updateEndpointUrl(server *httptest.Server) {
	self.queue.endpointUrl = server.URL + apiEndpoint
}

func (self *LogScaleQueueTestSuite) preparePayloads(payloads []*LogScalePayload) []byte {
	opts := vql_subsystem.EncOptsFromScope(self.scope)
	data, err := json.MarshalWithOptions(payloads, opts)
	require.NoError(self.T(), err)
	return data
}

func (self *LogScaleQueueTestSuite) TestPostBytesValid() {
	row := generateRow()
	timestamp, _ := functions.TimeFromAny(self.ctx, self.scope, testTimestampStringTZ)
	payloads := []*LogScalePayload{
		&LogScalePayload{
			Events: []LogScaleEvent{
				LogScaleEvent{
					Attributes: row,
					Timestamp:  timestamp,
				},
			},
			Tags: map[string]interface{}{
				"ClientId":       testClientId,
				"ClientHostname": testHostname,
			},
		},
	}

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	data := self.preparePayloads(payloads)

	resp, err := self.queue.postBytes(self.scope, data, len(payloads))
	require.NoError(self.T(), err)
	require.NotNil(self.T(), resp)

	retry, err := self.queue.shouldRetryRequest(self.ctx, resp, err)
	require.NoError(self.T(), err)
	require.False(self.T(), retry)
}

// Pointless but still valid
func (self *LogScaleQueueTestSuite) TestPostBytesEmpty() {
	payloads := []*LogScalePayload{}

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	data := self.preparePayloads(payloads)

	resp, err := self.queue.postBytes(self.scope, data, len(payloads))
	require.NoError(self.T(), err)
	require.NotNil(self.T(), resp)

	retry, err := self.queue.shouldRetryRequest(self.ctx, resp, err)
	require.NoError(self.T(), err)
	require.False(self.T(), retry)
}

func (self *LogScaleQueueTestSuite) TestPostBytesEmptyConnRefused() {
	payloads := []*LogScalePayload{}

	self.wantConnRefused = true

	err := self.queue.Open(self.ctx, self.scope, "http://localhost:1", validAuthToken)
	require.NoError(self.T(), err)

	data := self.preparePayloads(payloads)

	resp, err := self.queue.postBytes(self.scope, data, len(payloads))
	require.ErrorIs(self.T(), err, syscall.ECONNREFUSED)
	require.Nil(self.T(), resp)

	retry, err := self.queue.shouldRetryRequest(self.ctx, resp, err)
	require.NoError(self.T(), err)
	require.True(self.T(), retry)
}

func (self *LogScaleQueueTestSuite) TestPostBytesNoEvents() {
	payloads := []*LogScalePayload{
		&LogScalePayload{
			Tags: map[string]interface{}{
				"ClientId":       testClientId,
				"ClientHostname": testHostname,
			},
		},
	}

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	data := self.preparePayloads(payloads)
	resp, err := self.queue.postBytes(self.scope, data, len(payloads))
	require.NoError(self.T(), err)
	require.NotNil(self.T(), resp)
	require.Equal(self.T(), resp.StatusCode, http.StatusBadRequest)

	retry, err := self.queue.shouldRetryRequest(self.ctx, resp, err)
	require.False(self.T(), retry)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestPostEventsEmpty() {
	rows := []*ordereddict.Dict{}

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	err = self.queue.postEvents(self.ctx, self.scope, rows)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestPostEventsSingle() {
	rows := []*ordereddict.Dict{}

	rows = append(rows, generateRow())

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	err = self.queue.postEvents(self.ctx, self.scope, rows)
	require.Equal(self.T(), 1, len(rows))
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestPostEventsSingleConnRefused() {
	rows := []*ordereddict.Dict{}

	self.wantConnRefused = true

	rows = append(rows, generateRow())

	err := self.queue.Open(self.ctx, self.scope, "http://localhost:1", validAuthToken)
	require.NoError(self.T(), err)

	err = self.queue.postEvents(self.ctx, self.scope, rows)
	require.NotNil(self.T(), err)
	expectedErr := errMaxRetriesExceeded{}
	require.ErrorAs(self.T(), err, &expectedErr)
	require.ErrorIs(self.T(), err, syscall.ECONNREFUSED)
}

func (self *LogScaleQueueTestSuite) TestPostEventsMultiple() {
	rows := []*ordereddict.Dict{}

	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	err = self.queue.postEvents(self.ctx, self.scope, rows)
	require.NoError(self.T(), err)
}

func (self *LogScaleQueueTestSuite) TestPostEventsMultipleConnRefused() {
	rows := []*ordereddict.Dict{}

	self.wantConnRefused = true

	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())

	err := self.queue.Open(self.ctx, self.scope, "http://localhost:1", validAuthToken)
	require.NoError(self.T(), err)

	err = self.queue.postEvents(self.ctx, self.scope, rows)
	require.NotNil(self.T(), err)
	expectedErr := errMaxRetriesExceeded{}
	require.ErrorAs(self.T(), err, &expectedErr)
	require.ErrorIs(self.T(), err, syscall.ECONNREFUSED)
}

// Test whether events just make it into the queue properly
func (self *LogScaleQueueTestSuite) TestQueueEvents_Queued() {
	server := self.startMockServer()
	defer server.Close()

	// Special case: We want to do the processing ourselves
	self.queue.nWorkers = 0

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	rows := []*ordereddict.Dict{}

	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())

	for _, row := range rows {
		self.queue.QueueEvent(row)
	}

	require.Equal(self.T(), len(rows), int(atomic.LoadInt64(&self.queue.currentQueueDepth)))

	// Nothing is clearing the queue, so clear it so we don't get stuck during close
	for _, _ = range rows {
		<-self.queue.listener.Output()
	}
}

// Test whether events just make it back out of the queue and post properly
func (self *LogScaleQueueTestSuite) TestQueueEventsOpen_Dequeued() {
	server := self.startMockServer()
	defer server.Close()

	// Special case: We want to do the processing ourselves
	self.queue.nWorkers = 0

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	rows := []*ordereddict.Dict{}

	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())

	ctx, cancel := context.WithTimeout(self.ctx, time.Duration(1)*time.Second)

	// This is a worker.  It would've been started as part of Open()
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		count := 0
	L:
		for {
			select {
			case <-ctx.Done():
				break L
			case row, ok := <-self.queue.listener.Output():
				if !ok {
					break L
				}

				// Don't build up a list, just push one at a time for testing
				post := []*ordereddict.Dict{row}
				err = self.queue.postEvents(ctx, self.scope, post)
				require.NoError(self.T(), err)

				count += 1
			}
		}
		require.Equal(self.T(), len(rows), count)
	}()

	for _, row := range rows {
		self.queue.QueueEvent(row)
	}

	wg.Wait()
	require.Equal(self.T(), len(rows), int(atomic.LoadInt64(&self.queue.currentQueueDepth)))
	require.Equal(self.T(), 4, int(atomic.LoadInt64(&self.queue.postedEvents)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.failedEvents)))
	cancel()
}

// Test whether events just make it back out of the queue and are handled properly when the post fails
func (self *LogScaleQueueTestSuite) TestQueueEventsOpen_DequeuedFailure() {
	server := self.startMockServer()
	defer server.Close()

	// Special case: We want to do the processing ourselves
	self.queue.nWorkers = 0

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	rows := []*ordereddict.Dict{}

	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())

	ctx, cancel := context.WithTimeout(self.ctx, time.Duration(5)*time.Second)

	// This is a worker.  It would've been started as part of Open()
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		count := 0
	L:
		for {
			select {
			case <-ctx.Done():
				break L
			case row, ok := <-self.queue.listener.Output():
				if !ok {
					break L
				}
				atomic.AddInt64(&self.queue.currentQueueDepth, -1)

				// Can't use debugEvents since those are handled in processEvents
				if count == 2 {
					server.Close()
					server = self.startMockServerWithHandler(handler500)
					self.updateEndpointUrl(server)
				}

				// Don't build up a list, just push one at a time for testing
				post := []*ordereddict.Dict{row}
				err = self.queue.postEvents(ctx, self.scope, post)
				if count >= 2 {
					require.NotNil(self.T(), err)
					expectedErr := errMaxRetriesExceeded{}
					require.ErrorAs(self.T(), err, &expectedErr)
				} else {
					require.NoError(self.T(), err)
				}

				count += 1
			}
			if count >= len(rows) {
				break
			}
		}
		self.queue.Close(self.scope)
		require.Equal(self.T(), len(rows), count)
	}()

	for _, row := range rows {
		self.queue.QueueEvent(row)
	}

	wg.Wait()
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.currentQueueDepth)))
	require.Equal(self.T(), 2, int(atomic.LoadInt64(&self.queue.failedEvents)))
	require.Equal(self.T(), 2, int(atomic.LoadInt64(&self.queue.postedEvents)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.droppedEvents)))
	cancel()
	self.queue = nil
}

func (self *LogScaleQueueTestSuite) TestQueueEventsOpen_DequeuedConnRefused() {
	server := self.startMockServer()
	defer server.Close()

	// Special case: We want to do the processing ourselves
	self.queue.nWorkers = 0

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	rows := []*ordereddict.Dict{}

	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())
	rows = append(rows, generateRow())

	ctx, cancel := context.WithTimeout(self.ctx, time.Duration(5)*time.Second)

	// This is a worker.  It would've been started as part of Open()
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		count := 0
	L:
		for {
			select {
			case <-ctx.Done():
				break L
			case row, ok := <-self.queue.listener.Output():
				if !ok {
					break L
				}
				atomic.AddInt64(&self.queue.currentQueueDepth, -1)

				if count == 2 {
					self.queue.endpointUrl = "http://localhost:1" + apiEndpoint
				}

				// Don't build up a list, just push one at a time for testing
				post := []*ordereddict.Dict{row}
				err = self.queue.postEvents(ctx, self.scope, post)
				if count >= 2 {
					require.NotNil(self.T(), err)
					expectedErr := errMaxRetriesExceeded{}
					require.ErrorAs(self.T(), err, &expectedErr)
				} else {
					require.NoError(self.T(), err)
				}

				count += 1
			}
			if count >= len(rows) {
				break
			}
		}
		self.queue.Close(self.scope)
		require.Equal(self.T(), len(rows), count)
	}()

	for _, row := range rows {
		self.queue.QueueEvent(row)
	}

	wg.Wait()
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.currentQueueDepth)))
	require.Equal(self.T(), 2, int(atomic.LoadInt64(&self.queue.failedEvents)))
	require.Equal(self.T(), 2, int(atomic.LoadInt64(&self.queue.postedEvents)))
	cancel()
	self.queue = nil
}

func (self *LogScaleQueueTestSuite) TestProcessEvents_Working() {
	nRows := 100

	server := self.startMockServer()
	defer server.Close()

	err := self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	rows := []*ordereddict.Dict{}

	for i := 0; i < nRows; i += 1 {
		rows = append(rows, generateRow())
	}

	for _, row := range rows {
		self.queue.QueueEvent(row)
	}

	self.queue.Close(self.scope)

	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.currentQueueDepth)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.failedEvents)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.droppedEvents)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.totalRetries)))
	require.Equal(self.T(), nRows, int(atomic.LoadInt64(&self.queue.postedEvents)))
	self.queue = nil
}

func (self *LogScaleQueueTestSuite) TestProcessEvents_ShutdownWhileFailing() {
	nRows := 100

	server := self.startMockServer()
	defer server.Close()

	wg := sync.WaitGroup{}
	wg.Add(1)

	self.queue.SetEventBatchSize(1)
	err := self.queue.addDebugCallback(nRows/2, func(count int) {
		self.queue.endpointUrl = "http://localhost:1" + apiEndpoint
		wg.Done()
	})

	err = self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	rows := []*ordereddict.Dict{}

	for i := 0; i < nRows; i += 1 {
		rows = append(rows, generateRow())
	}

	require.NoError(self.T(), err)

	for _, row := range rows {
		self.queue.QueueEvent(row)
	}

	wg.Wait()
	self.queue.Close(self.scope)

	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.currentQueueDepth)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.totalRetries)))
	require.Equal(self.T(), (nRows/2)-1, int(atomic.LoadInt64(&self.queue.droppedEvents)))
	require.Equal(self.T(), nRows/2, int(atomic.LoadInt64(&self.queue.postedEvents)))
	require.Equal(self.T(), 1, int(atomic.LoadInt64(&self.queue.failedEvents)))
	self.queue = nil
}

func (self *LogScaleQueueTestSuite) TestProcessEvents_ShutdownAfterRecovery() {
	nRows := 100

	server := self.startMockServer()
	defer server.Close()

	self.queue.SetEventBatchSize(1)

	wg1 := sync.WaitGroup{}
	wg1.Add(1)
	err := self.queue.addDebugCallback(25, func(count int) {
		server.Close()
		server = self.startMockServerWithHandler(handler500)
		self.updateEndpointUrl(server)

	})

	err = self.queue.addDebugCallback(26, func(count int) {
		server.Close()
		server = self.startMockServer()
		self.updateEndpointUrl(server)

	})

	err = self.queue.addDebugCallback(99, func(count int) {
		wg1.Done()
	})

	err = self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	rows := []*ordereddict.Dict{}

	for i := 0; i < nRows; i += 1 {
		rows = append(rows, generateRow())
	}

	require.NoError(self.T(), err)

	for _, row := range rows {
		self.queue.QueueEvent(row)
	}

	wg1.Wait()

	self.queue.Close(self.scope)

	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.currentQueueDepth)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.droppedEvents)))
	require.Equal(self.T(), nRows-1, int(atomic.LoadInt64(&self.queue.postedEvents)))
	require.Equal(self.T(), 1, int(atomic.LoadInt64(&self.queue.totalRetries)))
	require.Equal(self.T(), 1, int(atomic.LoadInt64(&self.queue.failedEvents)))
	self.queue = nil
}

func (self *LogScaleQueueTestSuite) TestProcessEvents_4xx() {
	nRows := 100

	server := self.startMockServer()
	defer server.Close()

	self.queue.SetEventBatchSize(1)
	wg1 := sync.WaitGroup{}
	wg1.Add(1)
	err := self.queue.addDebugCallback(25, func(count int) {
		server.Close()
		server = self.startMockServerWithHandler(handler401)
		self.updateEndpointUrl(server)

	})

	err = self.queue.addDebugCallback(30, func(count int) {
		server.Close()
		server = self.startMockServer()
		self.updateEndpointUrl(server)

	})

	err = self.queue.addDebugCallback(99, func(count int) {
		wg1.Done()
	})

	err = self.queue.Open(self.ctx, self.scope, server.URL, validAuthToken)
	require.NoError(self.T(), err)

	rows := []*ordereddict.Dict{}

	for i := 0; i < nRows; i += 1 {
		rows = append(rows, generateRow())
	}

	require.NoError(self.T(), err)

	for _, row := range rows {
		self.queue.QueueEvent(row)
	}

	wg1.Wait()

	self.queue.Close(self.scope)

	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.currentQueueDepth)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.droppedEvents)))
	require.Equal(self.T(), 95, int(atomic.LoadInt64(&self.queue.postedEvents)))
	require.Equal(self.T(), 0, int(atomic.LoadInt64(&self.queue.totalRetries)))
	require.Equal(self.T(), 5, int(atomic.LoadInt64(&self.queue.failedEvents)))
	self.queue = nil
}

func TestLogScaleQueue(t *testing.T) {
	gMaxPoll = 1
	gMaxPollDev = 1
	suite.Run(t, new(LogScaleQueueTestSuite))
}

func (self *LogScaleQueue) addDebugCallback(count int, callback func(int)) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	if self.opened {
		return errQueueOpened
	}

	if !self.debugEventsEnabled {
		self.debugEventsMap = map[int][]func(int){}
		self.debugEventsEnabled = true
	}

	_, ok := self.debugEventsMap[count]
	if ok {
		self.debugEventsMap[count] = append(self.debugEventsMap[count], callback)
	} else {
		self.debugEventsMap[count] = []func(int){callback}
	}

	return nil
}
