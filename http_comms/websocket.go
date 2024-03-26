package http_comms

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/gorilla/websocket"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	writeWait = 10 * time.Second
)

var (
	notConnectedError = errors.New("WS Socket is not conencted")
)

// The websocket conenction is not thread safe so we need to
// synchronize it.
type Conn struct {
	*websocket.Conn

	mu sync.Mutex
}

func (self *Conn) WriteMessage(message_type int, message []byte) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Conn.WriteMessage(message_type, message)
}

type WebSocketConnection struct {
	from_server chan *http.Response
	to_server   chan []byte

	max_poll time.Duration

	mu         sync.Mutex
	cancel     func()
	ctx        context.Context
	config_obj *config_proto.Config
	ws         *Conn

	transport *http.Transport

	key string
}

func (self *WebSocketConnection) PumpMessagesToServer() {
	for {
		select {
		case <-self.ctx.Done():
			return

		case message, ok := <-self.to_server:
			if !ok {
				return
			}
			err := self.ws.WriteMessage(websocket.BinaryMessage, message)
			if err != nil {
				return
			}
		}
	}
}

func (self *WebSocketConnection) PumpMessagesFromServer(req *http.Request) {
	for {
		message_type, message, err := ReadMessageWithCtx(
			self.ws, self.ctx, self.config_obj)
		response := makeHTTPResponse(req, message_type, message, err)

		select {
		case <-self.ctx.Done():
			return

		case self.from_server <- response:
		}

		// If an error occured terminate the connections.
		if response.StatusCode != http.StatusOK {
			return
		}
	}
}

func (self *WebSocketConnection) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.cancel()
	self.ws.Close()
}

func (self *HTTPClientWithWebSocketTransport) NewWebSocketConnection(
	ctx context.Context,
	req *http.Request) (*WebSocketConnection, error) {
	max_poll := uint64(60)
	if self.config_obj.Client != nil &&
		self.config_obj.Client.MaxPoll > 0 {
		max_poll = self.config_obj.Client.MaxPoll
	}

	dialer := websocket.DefaultDialer
	dialer.TLSClientConfig = self.transport.TLSClientConfig

	key := req.URL.String()

	ws_, _, err := dialer.Dial(key, nil)
	if err != nil {
		return nil, err
	}

	ws := &Conn{Conn: ws_}

	ctx, cancel := context.WithCancel(ctx)

	res := &WebSocketConnection{
		// Emulate regular HTTP responses from the server, but these
		// are actually sent over the websocket connection. This
		// allows the transport to work with or without websocket
		// automatically.
		from_server: make(chan *http.Response),
		to_server:   make(chan []byte),
		cancel:      cancel,
		ctx:         ctx,
		config_obj:  self.config_obj,
		max_poll:    time.Duration(max_poll) * time.Second,
		transport:   self.transport,
		ws:          ws,
		key:         key,
	}

	// Log when ping messages arrive from the server. The server is
	// responsible for pinging the client periodically. If the
	// connection goes aways (e.g. network is dropped etc) then ping
	// messages wont get through and the read timeouts will be
	// triggered to tear the connection down.
	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	ws.SetPingHandler(func(message string) error {
		logger.Debug("Socket %v: Received Ping", res.key)

		deadline := utils.GetTime().Now().Add(PongPeriod(self.config_obj))

		// Extend the read and write timeouts when a ping arrives from
		// the server.
		ws.SetReadDeadline(deadline)
		ws.SetWriteDeadline(deadline)

		err := ws.WriteControl(websocket.PongMessage,
			[]byte(message), utils.GetTime().Now().Add(writeWait))
		if err == websocket.ErrCloseSent {
			return nil
		} else if _, ok := err.(net.Error); ok {
			return nil
		}

		// Update the nanny as we got a valid read message.
		self.nanny.UpdateReadFromServer()

		return err
	})

	// Pump messages from the remote server to the channel.
	go func() {
		defer self.removeConnection(req)

		res.PumpMessagesFromServer(req)
	}()

	// Pump messages from the channel to the remote server.
	go func() {
		defer self.removeConnection(req)

		res.PumpMessagesToServer()
	}()

	return res, nil
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

func (self *HTTPClientWithWebSocketTransport) removeConnection(req *http.Request) {
	self.mu.Lock()
	defer self.mu.Unlock()

	key := req.URL.String()
	conn, pres := self.ws_connections[key]
	if pres {
		conn.Close()
		delete(self.ws_connections, key)
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

		self.ws_connections[key] = conn
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
		data, _ = ioutil.ReadAll(req.Body)
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

	deadline := utils.GetTime().Now().Add(PongPeriod(config_obj))
	ws.SetReadDeadline(deadline)
	messageType, r, err := ws.NextReader()
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
