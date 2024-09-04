package http_comms

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/gorilla/websocket"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

var (
	WSConnectorFactory WebSocketConnectionFactory = WebSocketConnectionFactoryImpl{}
)

// The websocket connection is not thread safe so we need to
// synchronize it.
type Conn struct {
	*websocket.Conn

	mu sync.Mutex
}

// Control access to the underlying connection.
func (self *Conn) WriteMessageWithDeadline(
	message_type int, message []byte, deadline time.Time) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Conn.SetWriteDeadline(deadline)
	return self.Conn.WriteMessage(message_type, message)
}

func (self *Conn) WriteMessage(message_type int, message []byte) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Conn.WriteMessage(message_type, message)
}

type WebSocketConnection struct {
	id uint64

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
				logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
				logger.Error("WebSocketConnection: PumpMessagesToServer: %v\n", err)
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

		// If an error occured terminate the connection. Connection
		// will be removed and recreated by our caller.
		if response.StatusCode != http.StatusOK {
			return
		}
	}
}

func (self *WebSocketConnection) Id() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.id
}

func (self *WebSocketConnection) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.cancel()
	self.ws.Close()
}

// Make factory for WebSocketConnection so we can mock it for tests.
type WebSocketConnectionFactory interface {
	NewWebSocketConnection(
		ctx context.Context,
		self *HTTPClientWithWebSocketTransport,
		req *http.Request) (*WebSocketConnection, error)
}

type WebSocketConnectionFactoryImpl struct{}

func (WebSocketConnectionFactoryImpl) NewWebSocketConnection(
	ctx context.Context,
	self *HTTPClientWithWebSocketTransport,
	req *http.Request) (*WebSocketConnection, error) {
	max_poll := uint64(60)
	if self.config_obj.Client == nil {
		return nil, errors.New("No Client config available")
	}

	if self.config_obj.Client.MaxPoll > 0 {
		max_poll = self.config_obj.Client.MaxPoll
	}

	tls_config, err := networking.GetTlsConfig(self.config_obj.Client, "")
	if err != nil {
		return nil, err
	}

	// Need to create a new dialer with a new tlsConfig so it is not
	// shared with http dialer.
	// See https://github.com/gorilla/websocket/issues/601
	dialer := networking.MaybeSpyOnWSDialer(self.config_obj,
		&websocket.Dialer{
			Proxy:           GetProxy(),
			TLSClientConfig: tls_config,
		})

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
		id:          utils.GetId(),
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
		defer self.removeConnection(req, res.Id())

		res.PumpMessagesFromServer(req)
	}()

	// Pump messages from the channel to the remote server.
	go func() {
		defer self.removeConnection(req, res.Id())

		res.PumpMessagesToServer()
	}()

	return res, nil
}

type WebSocketConnectionFactoryForTests struct {
	WebSocketConnectionFactoryImpl

	mu sync.Mutex

	Connections map[string]*WebSocketConnection
}

func (self WebSocketConnectionFactoryForTests) NewWebSocketConnection(
	ctx context.Context,
	transport *HTTPClientWithWebSocketTransport,
	req *http.Request) (*WebSocketConnection, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	conn, err := self.WebSocketConnectionFactoryImpl.NewWebSocketConnection(
		ctx, transport, req)
	self.Connections[req.URL.Path] = conn
	return conn, err
}

func (self WebSocketConnectionFactoryForTests) Shutdown() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, k := range self.Connections {
		k.Close()
	}
}

func (self WebSocketConnectionFactoryForTests) GetConn(key string) (*WebSocketConnection, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	con, pres := self.Connections[key]
	return con, pres
}

func NewWebSocketConnectionFactoryForTests() WebSocketConnectionFactoryForTests {
	return WebSocketConnectionFactoryForTests{
		Connections: make(map[string]*WebSocketConnection),
	}
}
