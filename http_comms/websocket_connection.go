package http_comms

import (
	"context"
	"io"
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
	conn *websocket.Conn
	key  string

	// report stats in lock free way for the profile tracker.
	stats *ConnStats

	read_mu  sync.Mutex
	write_mu sync.Mutex
}

func NewWS(key string, ws *websocket.Conn) *Conn {
	res := &Conn{
		conn: ws,
		key:  key,
		stats: &ConnStats{
			create_time: utils.GetTime().Now(),
			id:          utils.GetId(),
		},
	}

	wsTracker.Register(key, res)

	return res
}

func (self *Conn) SetPingHandler(h func(message string) error) {
	self.conn.SetPingHandler(func(message string) error {
		self.stats.SetLastPing(utils.GetTime().Now())

		return h(message)
	})
}

func (self *Conn) SetPongHandler(config_obj *config_proto.Config) {
	self.conn.SetPongHandler(func(string) error {

		// On the server this is the best estimate of the last ping we
		// sent.
		self.stats.SetLastPing(utils.GetTime().Now())

		// Called with read_mu already held
		deadline := utils.Now().Add(PongPeriod(config_obj))
		return self._SetReadDeadline(deadline)
	})
}

func (self *Conn) Close() {
	// The Close and WriteControl methods can be called concurrently
	// with all other methods.
	self.conn.Close()

	wsTracker.Unregister(self.key)
}

func (self *Conn) SetReadDeadline(deadline time.Time) error {
	self.read_mu.Lock()
	defer self.read_mu.Unlock()

	self.stats.SetReadLock(true)
	defer self.stats.SetReadLock(false)

	return self._SetReadDeadline(deadline)
}

func (self *Conn) _SetReadDeadline(deadline time.Time) error {
	stats := self.stats.Get()

	if deadline.Before(stats.read_deadline) {
		return nil
	}

	self.stats.SetReadDeadline(deadline)
	return self.conn.SetReadDeadline(deadline)
}

func (self *Conn) GetStats() *ConnStats {
	return self.stats.Get()
}

func (self *Conn) SetWriteDeadline(deadline time.Time) error {
	self.write_mu.Lock()
	defer self.write_mu.Unlock()

	self.stats.SetWriteLock(true)
	defer self.stats.SetWriteLock(false)

	return self._SetWriteDeadline(deadline)
}

func (self *Conn) _SetWriteDeadline(deadline time.Time) error {
	stats := self.stats.Get()

	if deadline.Before(stats.write_deadline) {
		return nil
	}

	self.stats.SetWriteDeadline(deadline)
	return self.conn.SetWriteDeadline(deadline)
}

func (self *Conn) ReadMessage() (messageType int, p []byte, err error) {
	self.read_mu.Lock()
	defer self.read_mu.Unlock()

	self.stats.SetReadLock(true)
	defer self.stats.SetReadLock(false)

	return self.conn.ReadMessage()
}

func (self *Conn) SetReadLimit(limit int64) {
	self.read_mu.Lock()
	defer self.read_mu.Unlock()

	self.stats.SetReadLock(true)
	defer self.stats.SetReadLock(false)

	self.conn.SetReadLimit(limit)
}

func (self *Conn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	// The Close and WriteControl methods can be called concurrently
	// with all other methods.
	return self.conn.WriteControl(messageType, data, deadline)
}

func (self *Conn) NextReaderWithDeadline(deadline time.Time) (int, io.Reader, error) {
	self.read_mu.Lock()
	defer self.read_mu.Unlock()

	self.stats.SetReadLock(true)
	defer self.stats.SetReadLock(false)

	err := self._SetReadDeadline(deadline)
	if err != nil {
		return 0, nil, err
	}

	return self.conn.NextReader()
}

// Control access to the underlying connection.
func (self *Conn) WriteMessageWithDeadline(
	message_type int, message []byte, deadline time.Time) error {
	self.write_mu.Lock()
	defer self.write_mu.Unlock()

	self.stats.SetWriteLock(true)
	defer self.stats.SetWriteLock(false)

	err := self._SetWriteDeadline(deadline)
	if err != nil {
		return err
	}
	return self.conn.WriteMessage(message_type, message)
}

func (self *Conn) WriteMessage(message_type int, message []byte) error {
	self.write_mu.Lock()
	defer self.write_mu.Unlock()

	self.stats.SetWriteLock(true)
	defer self.stats.SetWriteLock(false)

	return self.conn.WriteMessage(message_type, message)
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
	if self.cancel != nil {
		self.cancel()
	}
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

	// Log when ping messages arrive from the server. The server is
	// responsible for pinging the client periodically. If the
	// connection goes aways (e.g. network is dropped etc) then ping
	// messages wont get through and the read timeouts will be
	// triggered to tear the connection down.
	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)

	ws := NewWS(key, ws_)

	ws.SetPingHandler(func(message string) error {
		logger.Debug("Socket %v: Received Ping", key)

		deadline := utils.GetTime().Now().Add(PongPeriod(self.config_obj))

		// Extend the read and write timeouts when a ping arrives from
		// the server.

		// This ping handler is called from NextReaderWithDeadline and
		// so it is already holding the read lock.
		_ = ws._SetReadDeadline(deadline)

		// We must set the write deadline with a lock since we are
		// called with the read lock.
		_ = ws.SetWriteDeadline(deadline)

		err = ws_.WriteControl(websocket.PongMessage,
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

	// Pump messages from the remote server to the channel.
	go func() {
		defer ws.Close()
		defer self.removeConnection(req, res.Id())

		res.PumpMessagesFromServer(req)
	}()

	// Pump messages from the channel to the remote server.
	go func() {
		defer ws.Close()
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

func (self *WebSocketConnectionFactoryForTests) NewWebSocketConnection(
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

func (self *WebSocketConnectionFactoryForTests) Shutdown() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, k := range self.Connections {
		k.Close()
	}
}

func (self *WebSocketConnectionFactoryForTests) GetConn(key string) (*WebSocketConnection, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	con, pres := self.Connections[key]
	return con, pres
}

func NewWebSocketConnectionFactoryForTests() *WebSocketConnectionFactoryForTests {
	return &WebSocketConnectionFactoryForTests{
		Connections: make(map[string]*WebSocketConnection),
	}
}
