package http_comms

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/faults"
)

const (
	writeWait = 10 * time.Second
)

var (
	notConnectedError = errors.New("WS Socket is not connected")

	currentWsConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "client_comms_current_outgoing_ws_sockets",
		Help: "Number of currently connected ws connections.",
	}, []string{"url"})
)

func (self *HTTPClientWithWebSocketTransport) NewWebSocketConnection(
	ctx context.Context,
	req *http.Request) (*WebSocketConnection, error) {
	return WSConnectorFactory.NewWebSocketConnection(ctx, self, req)
}

// Implements http.RoundTripper
type HTTPClientWithWebSocketTransport struct {
	mu             sync.Mutex
	ws_connections map[string]*WebSocketConnection
	config_obj     *config_proto.Config

	transport *http.Transport
	nanny     *executor.NannyService
}

func (self *HTTPClientWithWebSocketTransport) RoundTrip(
	req *http.Request) (*http.Response, error) {
	switch req.URL.Scheme {
	case "ws", "wss":
		return self.roundTripWS(req)
	default:
		return self.transport.RoundTrip(req)
	}
}

func (self *HTTPClientWithWebSocketTransport) removeConnection(
	req *http.Request, id uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	key := req.URL.String()
	conn, pres := self.ws_connections[key]
	if pres && conn.Id() == id {
		logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
		logger.Debug(
			"HTTPClientWithWebSocketTransport: Uninstalling connector %v for %v",
			conn.Id(), key)
		conn.Close()
		delete(self.ws_connections, key)
		currentWsConnections.With(prometheus.Labels{"url": key}).Dec()
	}
}

func (self *HTTPClientWithWebSocketTransport) getConnection(
	req *http.Request) (res *WebSocketConnection, err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	ctx := req.Context()
	key := req.URL.String()
	conn, pres := self.ws_connections[key]
	if !pres {
		conn, err = self.NewWebSocketConnection(ctx, req)
		if err != nil {
			return nil, err
		}
		logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
		logger.Debug(
			"HTTPClientWithWebSocketTransport: Installing connector %v to %v",
			conn.Id(), key)
		self.ws_connections[key] = conn
		currentWsConnections.With(prometheus.Labels{"url": key}).Inc()
	}
	return conn, nil
}

func (self *HTTPClientWithWebSocketTransport) roundTripWS(
	req *http.Request) (resp *http.Response, err error) {

	conn, err := self.getConnection(req)
	if err != nil {
		return nil, err
	}

	// Write the request on the channel
	var data []byte
	if req.Body != nil {
		data, _ = utils.ReadAllWithLimit(req.Body, constants.MAX_MEMORY)
		faults.FaultInjector.BlockHTTPDo(req.Context())
	}

	select {
	case <-conn.ctx.Done():
		return nil, notConnectedError

	case conn.to_server <- data:
	}

	// Read the response from the channel
	select {
	case <-conn.ctx.Done():
		// Connection is dead
		return nil, notConnectedError

	case response, ok := <-conn.from_server:
		if !ok {
			return nil, notConnectedError
		}

		// Return the emulated response.
		return response, nil
	}
}

func NewHTTPClient(
	config_obj *config_proto.Config,
	transport *http.Transport,
	nanny *executor.NannyService) *http.Client {
	return &http.Client{
		// Let us handle redirect ourselves.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &HTTPClientWithWebSocketTransport{
			transport:      transport,
			config_obj:     config_obj,
			ws_connections: make(map[string]*WebSocketConnection),
			nanny:          nanny,
		},
	}
}

// Emulate a http.Response from the websocket message we
// received. This allows callers to easily switch between http and
// websocket connections.
func makeHTTPResponse(
	req *http.Request,
	message_type int,
	message []byte, err error) *http.Response {

	if err != nil {
		// Tell the caller they need to retry. The connection is closed so
		// we try to open it again.
		if errors.Is(err, net.ErrClosed) ||
			strings.Contains(err.Error(), "websocket: close 1006 (abnormal closure)") {
			return &http.Response{
				Status:     err.Error(),
				StatusCode: http.StatusRequestTimeout,
				Request:    req,
			}
		}

		return &http.Response{
			Status:     err.Error(),
			StatusCode: http.StatusServiceUnavailable,
			Request:    req,
		}
	}

	// The server is trying to send us an error message
	switch message_type {
	case websocket.BinaryMessage, websocket.TextMessage:
		msg := &crypto.WSErrorMessage{}
		err := json.Unmarshal(message, msg)
		if err != nil {
			return &http.Response{
				Status:     err.Error(),
				StatusCode: http.StatusServiceUnavailable,
				Request:    req,
			}
		}

		return &http.Response{
			Status:     msg.Error,
			StatusCode: msg.HTTPCode,
			Body:       io.NopCloser(bytes.NewReader(msg.Data)),
			Request:    req,
		}

	default:
		return &http.Response{
			Status:     "500 Unknown WS message",
			StatusCode: http.StatusServiceUnavailable,
			Request:    req,
		}
	}
}

func ReadMessageWithCtx(
	ws *Conn,
	ctx context.Context,
	config_obj *config_proto.Config) (
	messageType int, p []byte, err error) {

	buffer := &bytes.Buffer{}

	deadline := utils.Now().Add(PongPeriod(config_obj))
	messageType, r, err := ws.NextReaderWithDeadline(deadline)
	if err != nil {
		return messageType, nil, err
	}
	_, err = utils.Copy(ctx, buffer, r)
	return messageType, buffer.Bytes(), err
}

// Server will ping periodically.
func PingWait(
	config_obj *config_proto.Config) time.Duration {
	if config_obj.Client != nil &&
		config_obj.Client.WsPingWaitSec > 0 {
		return time.Duration(config_obj.Client.WsPingWaitSec) * time.Second
	}

	// Default 60 sec.
	return 60 * time.Second
}

// Time allowed to read the next pong message from the peer - a bit
// more than the period of pings so we can be sure that the server has
// really gone away.
func PongPeriod(config_obj *config_proto.Config) time.Duration {
	return (PingWait(config_obj) * 11) / 10
}
