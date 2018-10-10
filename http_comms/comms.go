package http_comms

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/logging"
)

// Responsible for maybe enrolling the client. Enrollments should not
// be done too frequently and should only be done in response for the
// 406 HTTP codes.
type Enroller struct {
	config_obj              *config.Config
	manager                 *crypto.CryptoManager
	executor                executor.Executor
	logger                  *logging.Logger
	last_enrollment_time    time.Time
	last_foreman_check_time time.Time
}

// TODO: This is a hold over from GRR - do we need it?  GRR's
// enrollments are very slow and launching flows very expensive so it
// makes sense to delay this. Velociraptor's enrollments are very
// cheap so perhaps we dont need to worry about it here?
func (self *Enroller) MaybeEnrol() {
	// Only send an enrolment request at most every minute so as
	// not to overwhelm the server if it can not keep up.
	if time.Now().After(
		self.last_enrollment_time.Add(1 * time.Minute)) {
		csr_pem, err := self.manager.GetCSR()
		if err != nil {
			return
		}

		csr := &crypto_proto.Certificate{
			Type: crypto_proto.Certificate_CSR,
			Pem:  csr_pem,
		}

		reply := &crypto_proto.GrrMessage{
			SessionId:   constants.ENROLLMENT_WELL_KNOWN_FLOW,
			ArgsRdfName: "Certificate",
			Priority:    crypto_proto.GrrMessage_HIGH_PRIORITY,
			ClientType:  crypto_proto.GrrMessage_VELOCIRAPTOR,
		}

		serialized_csr, err := proto.Marshal(csr)
		if err != nil {
			return
		}

		reply.Args = serialized_csr

		self.last_enrollment_time = time.Now()
		self.logger.Info("Enrolling")
		go func() {
			self.executor.SendToServer(reply)
		}()
	}
}

// Velociraptor's foreman is very quick (since we just compare the
// last hunt timestamp the client provides to the server's last hunt
// timestamp) so it is ok to send a foreman message in every receiver.
func (self *Enroller) GetMessageList() *crypto_proto.MessageList {
	reply := &crypto_proto.GrrMessage{
		SessionId:   constants.FOREMAN_WELL_KNOWN_FLOW,
		ArgsRdfName: "ForemanCheckin",
		Priority:    crypto_proto.GrrMessage_LOW_PRIORITY,
		ClientType:  crypto_proto.GrrMessage_VELOCIRAPTOR,
	}

	serialized_arg, err := proto.Marshal(&actions_proto.ForemanCheckin{
		LastHuntTimestamp: self.config_obj.Writeback.HuntLastTimestamp,
	})
	if err != nil {
		return &crypto_proto.MessageList{}
	}
	reply.Args = serialized_arg

	result := &crypto_proto.MessageList{}
	result.Job = append(result.Job, reply)

	return result
}

// Responsible for using HTTP to talk with the end point.
type HTTPConnector struct {
	// The Crypto Manager for communicating with the current
	// URL. Note, when the URL is changed, the CryptoManager is
	// initialized by a successful connection to the URL's
	// server.pem endpoint.
	manager *crypto.CryptoManager
	logger  *logging.Logger

	minPoll, maxPoll time.Duration

	// Used to cycle through the urls slice.
	mu              sync.Mutex
	current_url_idx int
	urls            []string

	client *http.Client

	// Obtained from the server's Cert CommonName.
	server_name string
}

func NewHTTPConnector(
	config_obj *config.Config,
	manager *crypto.CryptoManager,
	logger *logging.Logger) *HTTPConnector {

	max_poll := config_obj.Client.MaxPoll
	if max_poll == 0 {
		max_poll = 60
	}

	return &HTTPConnector{
		manager: manager,
		logger:  logger,

		minPoll: time.Duration(1) * time.Second,
		maxPoll: time.Duration(max_poll) * time.Second,

		urls: config_obj.Client.ServerUrls,

		client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
					DualStack: true,
				}).DialContext,
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          100,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
			},
		},
	}

}

func (self *HTTPConnector) GetCurrentUrl() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.urls[self.current_url_idx]
}

func (self *HTTPConnector) Post(handler string, data []byte) (
	*http.Response, error) {

	reader := bytes.NewReader(data)
	resp, err := self.client.Post(self.GetCurrentUrl()+handler,
		"application/binary", reader)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return resp, nil
}

// Contact the server and verify its public key. May block
// indefinitely until a valid trusted server is found. After this
// function completes the current URL is pointed at a valid server
// which should be used for all further Post() optations.  Note that
// this function holds a lock on the connector for the duration of the
// call. All other POST operations will be blocked until a valid
// server is found.
func (self *HTTPConnector) ReKeyNextServer() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for {
		err := self.rekeyNextServer()
		if err == nil {
			return
		}

		select {
		case <-time.After(self.maxPoll):
			continue
		}
	}
}

func (self *HTTPConnector) ServerName() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.server_name
}

func (self *HTTPConnector) rekeyNextServer() error {
	// Advance the current URL to the next one in line.
	if self.server_name != "" {
		self.current_url_idx = ((self.current_url_idx + 1) % len(self.urls))
	}

	// Try to fetch the server pem.
	url := self.urls[self.current_url_idx]
	resp, err := self.client.Get(url + "server.pem")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	pem, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.WithStack(err)
	}

	// This will replace the current server_name
	// certificate in the manager.
	server_name, err := self.manager.AddCertificate(pem)
	if err != nil {
		return err
	}
	self.server_name = *server_name
	self.logger.Info("Received PEM for %v from %v", self.server_name, url)

	return nil
}

// Manages reading jobs from the reader notification channel.
type NotificationReader struct {
	config_obj *config.Config
	connector  *HTTPConnector
	manager    *crypto.CryptoManager
	executor   executor.Executor
	enroller   *Enroller
	handler    string
	logger     *logging.Logger
	name       string

	minPoll, maxPoll      time.Duration
	current_poll_duration time.Duration
	IsPaused              int32
}

func NewNotificationReader(
	config_obj *config.Config,
	connector *HTTPConnector,
	manager *crypto.CryptoManager,
	executor executor.Executor,
	enroller *Enroller,
	logger *logging.Logger,
	name string,
	handler string) *NotificationReader {
	return &NotificationReader{
		config_obj: config_obj,
		connector:  connector,
		manager:    manager,
		executor:   executor,
		enroller:   enroller,
		name:       name,
		handler:    handler,
		logger:     logger,
		minPoll:    time.Duration(1) * time.Second,
		maxPoll:    time.Duration(config_obj.Client.MaxPoll) * time.Second,

		current_poll_duration: time.Second,
	}
}

// Block until the messages are sent. Will retry, back off and rekey
// the server.
func (self *NotificationReader) sendMessageList(
	ctx context.Context, message_list *crypto_proto.MessageList) {
	for {
		if atomic.LoadInt32(&self.IsPaused) == 0 {
			err := self.sendToURL(ctx, message_list)
			// Success!
			if err == nil {
				return
			}

			// Failed to fetch the URL - This could happen because
			// the server is overloaded, or the client is off the
			// network. We need to back right off and retry the
			// POST again.
			self.logger.Info("Failed to fetch URL %v: %v",
				self.connector.GetCurrentUrl()+self.handler, err)
		}

		select {
		case <-ctx.Done():
			return

			// Wait for the maximum length of time
			// and try to rekey the next URL.
		case <-time.After(self.maxPoll):
			self.connector.ReKeyNextServer()
			continue
		}
	}
}

func (self *NotificationReader) sendToURL(
	ctx context.Context, message_list *crypto_proto.MessageList) error {

	if self.connector.ServerName() == "" {
		self.connector.ReKeyNextServer()
	}

	self.logger.Info("%s: Connected to %s", self.name,
		self.connector.GetCurrentUrl()+self.handler)

	cipher_text, err := self.manager.EncryptMessageList(
		message_list, self.connector.ServerName())
	if err != nil {
		return err
	}

	resp, err := self.connector.Post(self.handler, cipher_text)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Enrollment is pretty quick so we need to retry sooner -
	// return no error so the next poll happens in minPoll.
	if resp.StatusCode == 406 {
		self.enroller.MaybeEnrol()
		return nil
	}

	self.logger.Info("%s: sent %d bytes, response with status: %v",
		self.name, len(cipher_text), resp.Status)
	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}

	encrypted := []byte{}
	buf := make([]byte, 4096)

	// We need to be able to cancel the read here so we do not use
	// ioutil.ReadAll()
process_response:
	for {
		select {
		case <-ctx.Done():
			return errors.New("Cancelled")

		default:
			n, err := resp.Body.Read(buf)
			if err != nil && err != io.EOF {
				return errors.WithStack(err)
			}
			if n == 0 || err == io.EOF {
				break process_response
			}

			encrypted = append(encrypted, buf[:n]...)
		}
	}

	response_message_list, err := self.manager.DecryptMessageList(encrypted)
	if err != nil {
		return err
	}

	for _, msg := range response_message_list.Job {
		self.executor.ProcessRequest(msg)
	}

	return nil
}

// The Receiver channel is used to receive commands from the server:
// 1. We send an empty MessageList{} with a POST
//    (but this allows us to authenticate to the server).
// 2. Block on reading the body of the POST until the server completes
//    the request.  The server will trickle feed the connection with
//    data to keep it alive for any intermediate proxies.
// 3. Any received messages will be processed automatically by
//    self.sendMessageList()
// 4. If there are errors, we back off and wait for self.maxPoll.
func (self *NotificationReader) Start(ctx context.Context) {
	go func() {
		for {
			// The Reader does not send any server bound
			// messages - it is blocked reading server
			// responses.
			self.sendMessageList(ctx, self.GetMessageList())

			select {
			case <-ctx.Done():
				return

			case <-time.After(self.minPoll):
				continue
			}
		}
	}()
}

// Velociraptor's foreman is very quick (since it is just an int
// comparison between the client's last hunt timestamp and the
// server's last hunt timestamp). It is therefore ok to send a foreman
// message in every reader message to improve hunt latency.
func (self *NotificationReader) GetMessageList() *crypto_proto.MessageList {
	reply := &crypto_proto.GrrMessage{
		SessionId:   constants.FOREMAN_WELL_KNOWN_FLOW,
		ArgsRdfName: "ForemanCheckin",
		Priority:    crypto_proto.GrrMessage_LOW_PRIORITY,
		ClientType:  crypto_proto.GrrMessage_VELOCIRAPTOR,
	}

	serialized_arg, err := proto.Marshal(&actions_proto.ForemanCheckin{
		LastHuntTimestamp: self.config_obj.Writeback.HuntLastTimestamp,
	})
	if err != nil {
		return &crypto_proto.MessageList{}
	}
	reply.Args = serialized_arg

	result := &crypto_proto.MessageList{}
	result.Job = append(result.Job, reply)

	return result
}

type Sender struct {
	*NotificationReader
	pending_messages chan *crypto_proto.GrrMessage
}

// The sender simply sends any server bound messages to the server. We
// only send messages when responses are pending.
func (self *Sender) Start(ctx context.Context) {
	// Pump messages from the executor to the pending message list
	// - this is our local queue of output pending messages.
	go func() {
		for {
			// If we are paused we sleep here. Note that
			// by not reading the executor we block it and
			// therefore all processing should pause
			// (since the executor channel itself has no
			// buffer).
			if atomic.LoadInt32(&self.IsPaused) != 0 {
				select {
				case <-time.After(self.minPoll):
					continue
				}
			} else {
				select {
				case <-ctx.Done():
					return

				case msg := <-self.executor.ReadResponse():
					// Executor closed the channel.
					if msg == nil {
						close(self.pending_messages)
						return
					}

					self.pending_messages <- msg
				}
			}
		}
	}()

	go func() {
		for {
			if atomic.LoadInt32(&self.IsPaused) == 0 {
				// If there is some data in the queues we send
				// it immediately. If there is no data pending
				// we send nothing.
				message_list := self.drainMessageQueue()
				if len(message_list.Job) > 0 {
					self.sendMessageList(ctx, message_list)

					// We need to make sure our
					// memory footprint is as
					// small as possible. The
					// Velociraptor client
					// prioritizes low memory
					// footprint over latency. We
					// just sent data to the
					// server and we wont need
					// that for a while so we can
					// free our memory to the OS.
					debug.FreeOSMemory()
				}
			}
			// Wait a minimum time before sending the next
			// one to give the executor a chance to fill
			// the queue.
			select {
			case <-ctx.Done():
				return

			case <-time.After(self.minPoll):
				continue
			}

		}
	}()
}

// Pull off as many messages as we can off the channel to send. Note:
// As we drain the channel the executor will be woken to fill it up
// again - since the self.pending_messages channel has a buffer.
func (self *Sender) drainMessageQueue() *crypto_proto.MessageList {
	result := &crypto_proto.MessageList{}

	for {
		select {
		case item := <-self.pending_messages:
			result.Job = append(result.Job, item)

		default:
			// No blocking - if there is no messages
			// available, just return
			return result
		}
	}
}

func NewSender(
	config_obj *config.Config,
	connector *HTTPConnector,
	manager *crypto.CryptoManager,
	executor executor.Executor,
	enroller *Enroller,
	logger *logging.Logger,
	name string,
	handler string) *Sender {
	result := &Sender{
		NewNotificationReader(config_obj, connector, manager,
			executor, enroller, logger, name, handler),

		// Allow the executor to queue 100 messages in the same packet.
		make(chan *crypto_proto.GrrMessage, 100),
	}

	return result

}

type HTTPCommunicator struct {
	config_obj *config.Config

	logger *logging.Logger

	// Read jobs from the servers notification channel.
	receiver *NotificationReader

	// Potentially enrols the client.
	enroller *Enroller

	// Sends results back to the server.
	sender *Sender
}

func (self *HTTPCommunicator) SetPause(is_paused bool) {
	value := int32(0)
	if is_paused {
		value = 1
	}
	atomic.StoreInt32(&self.sender.IsPaused, value)
	atomic.StoreInt32(&self.receiver.IsPaused, value)
}

// Run forever.
func (self *HTTPCommunicator) Run(ctx context.Context) {
	self.logger.Info("Starting HTTPCommunicator: %v", self.receiver.connector.urls)

	self.receiver.Start(ctx)
	self.sender.Start(ctx)

	select {
	case <-ctx.Done():
		return
	}
}

func NewHTTPCommunicator(
	config_obj *config.Config,
	manager *crypto.CryptoManager,
	executor executor.Executor,
	urls []string) (*HTTPCommunicator, error) {

	logger := logging.NewLogger(config_obj)
	enroller := &Enroller{
		config_obj: config_obj,
		manager:    manager,
		executor:   executor,
		logger:     logger}
	connector := NewHTTPConnector(config_obj, manager, logger)

	result := &HTTPCommunicator{
		config_obj: config_obj,
		logger:     logger,
		enroller: &Enroller{
			config_obj: config_obj,
			manager:    manager,
			executor:   executor,
			logger:     logger,
		},
		sender: NewSender(
			config_obj, connector, manager, executor, enroller,
			logger, "Sender", "control"),
		receiver: NewNotificationReader(
			config_obj, connector, manager, executor, enroller,
			logger, "Receiver", "reader"),
	}

	return result, nil
}
