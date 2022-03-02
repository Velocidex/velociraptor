/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// Server sent a redirect message.
	RedirectError = errors.New("RedirectError")

	// Can be sent from connector's Post() when the server requires
	// enrolment (sending HTTP 406 status).
	EnrolError = errors.New("EnrolError")

	mu   sync.Mutex
	Rand func(int) int = rand.Intn

	proxyHandler = http.ProxyFromEnvironment
)

// Responsible for maybe enrolling the client. Enrollments should not
// be done too frequently and should only be done in response for the
// 406 HTTP codes.
type Enroller struct {
	config_obj           *config_proto.Config
	manager              crypto.ICryptoManager
	executor             executor.Executor
	logger               *logging.LogContext
	last_enrollment_time time.Time
	clock                utils.Clock
}

// TODO: This is a hold over from GRR - do we need it?  GRR's
// enrollments are very slow and launching flows very expensive so it
// makes sense to delay this. Velociraptor's enrollments are very
// cheap so perhaps we dont need to worry about it here?
func (self *Enroller) MaybeEnrol() {
	// Only send an enrolment request at most every minute so as
	// not to overwhelm the server if it can not keep up.
	if self.clock.Now().After(
		self.last_enrollment_time.Add(1 * time.Minute)) {
		csr_pem, err := self.manager.GetCSR()
		if err != nil {
			return
		}

		self.last_enrollment_time = time.Now()
		self.logger.Info("Enrolling")

		go self.executor.SendToServer(&crypto_proto.VeloMessage{
			SessionId: constants.ENROLLMENT_WELL_KNOWN_FLOW,
			CSR: &crypto_proto.Certificate{
				Type: crypto_proto.Certificate_CSR,
				Pem:  csr_pem,
			},
			// Enrolment messages should be sent
			// immediately and not queued client side.
			Urgent: true,
		})
	}
}

// Connectors abstract the http.Post() operation. Make an interface so
// it can be mocked.
type IConnector interface {
	GetCurrentUrl(handler string) string
	Post(ctx context.Context,
		name string, // Name of the component calling Post (used for logging)
		handler string, // The URL handler we post to
		data []byte, priority bool) (*bytes.Buffer, error)
	ReKeyNextServer(ctx context.Context)
	ServerName() string
}

// Responsible for using HTTP to talk with the end point.
type HTTPConnector struct {
	config_obj *config_proto.Config

	// The Crypto Manager for communicating with the current
	// URL. Note, when the URL is changed, the CryptoManager is
	// initialized by a successful connection to the URL's
	// server.pem endpoint.
	manager crypto.IClientCryptoManager
	logger  *logging.LogContext

	minPoll, maxPoll time.Duration
	maxPollDev       uint64
	// Used to cycle through the urls slice.
	mu               sync.Mutex
	current_url_idx  int
	last_success_idx int
	urls             []string

	client *http.Client

	// Obtained from the server's Cert CommonName.
	server_name string

	// If the last request caused a redirect, we switch to that
	// server immediately and keep accessing that server until the
	// an error occurs or we are further redirected.
	redirect_to_server int

	clock utils.Clock
}

func NewHTTPConnector(
	config_obj *config_proto.Config,
	manager crypto.IClientCryptoManager,
	logger *logging.LogContext,
	urls []string,
	clock utils.Clock) (*HTTPConnector, error) {

	if config_obj.Client == nil {
		return nil, errors.New("Client not configured")
	}

	max_poll := config_obj.Client.MaxPoll
	if max_poll == 0 {
		max_poll = 60
	}

	maxPollDev := config_obj.Client.MaxPollStd
	if maxPollDev == 0 {
		maxPollDev = 30
	}

	CA_Pool := x509.NewCertPool()
	err := crypto.AddDefaultCerts(config_obj.Client, CA_Pool)
	if err != nil {
		return nil, err
	}

	tls_config := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(100),
		RootCAs:            CA_Pool,
	}

	// For self signed certificates we must ignore the server name
	// and only trust certs issued by our server.
	if config_obj.Client.UseSelfSignedSsl {
		logger.Info("Expecting self signed certificate for server.")

		// We only trust **our** pinned server name for HTTP comms.
		// NOTE: This stops an api cert from being presented for the
		// server. This setting also allows the server to be accessed
		// by e.g. localhost despite the certificate being issued to
		// VelociraptorServer.
		tls_config.ServerName = config_obj.Client.PinnedServerName
	} else {

		// Not self signed - add the public roots for verifications.
		crypto.AddPublicRoots(tls_config.RootCAs)
	}

	timeout := config_obj.Client.ConnectionTimeout
	if timeout == 0 {
		timeout = 300 // 5 Min default
	}

	self := &HTTPConnector{
		config_obj: config_obj,
		manager:    manager,
		logger:     logger,
		clock:      clock,

		// Start with a random URL from the set of
		// preconfigured URLs. This should distribute clients
		// randomly to all frontends.
		current_url_idx: GetRand()(len(urls)),

		minPoll:    time.Duration(1) * time.Second,
		maxPoll:    time.Duration(max_poll) * time.Second,
		maxPollDev: maxPollDev,

		urls: urls,

		client: &http.Client{
			// Let us handle redirect ourselves.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   time.Duration(timeout) * time.Second,
					KeepAlive: time.Duration(timeout) * time.Second,
					DualStack: true,
				}).DialContext,
				Proxy:                 proxyHandler,
				MaxIdleConns:          100,
				IdleConnTimeout:       time.Duration(timeout) * time.Second,
				TLSHandshakeTimeout:   time.Duration(timeout) * time.Second,
				ExpectContinueTimeout: time.Duration(timeout) * time.Second,
				ResponseHeaderTimeout: time.Duration(timeout) * time.Second,
				TLSClientConfig:       tls_config,
			},
		},
	}

	return self, nil

}

func (self *HTTPConnector) GetCurrentUrl(handler string) string {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.redirect_to_server > 0 {
		self.redirect_to_server--
		return self.urls[self.current_url_idx] + handler + "?r=1"
	}

	return self.urls[self.current_url_idx] + handler
}

func (self *HTTPConnector) Post(
	ctx context.Context, name, handler string,
	data []byte, urgent bool) (*bytes.Buffer, error) {

	reader := bytes.NewReader(data)
	req, err := http.NewRequestWithContext(ctx,
		"POST", self.GetCurrentUrl(handler), reader)
	if err != nil {
		self.logger.Info("Post to %v returned %v - advancing to next server\n",
			self.GetCurrentUrl(handler), err)
		self.advanceToNextServer(ctx)
		return nil, errors.WithStack(err)
	}

	trace := &httptrace.ClientTrace{
		GotConn: func(connInfo httptrace.GotConnInfo) {
			self.logger.WithFields(logrus.Fields{
				"LocalAddr": connInfo.Conn.LocalAddr(),
				"Reused":    connInfo.Reused,
				"WasIdle":   connInfo.WasIdle,
				"IdleTime":  connInfo.IdleTime,
			}).Debug("Connection Info")
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	req.Header.Set("User-Agent", constants.USER_AGENT)
	req.Header.Set("Content-Type", "application/binary")
	if urgent {
		req.Header.Set("X-Priority", "urgent")
	}

	resp, err := self.client.Do(req)
	if err != nil && err != io.EOF {
		self.logger.Info("Post to %v returned %v - advancing to next server\n",
			self.GetCurrentUrl(handler), err)

		// POST error - rotate to next URL
		self.advanceToNextServer(ctx)
		return nil, errors.WithStack(err)
	}
	// Must make sure to close the body or we leak sockets.
	defer resp.Body.Close()

	self.logger.Info("%s: sent %d bytes, response with status: %v",
		name, len(data), resp.Status)

	// Handle redirect. Frontends may redirect us to other
	// frontends.
	switch resp.StatusCode {
	case 301:
		dest, pres := resp.Header["Location"]
		if !pres || len(dest) == 0 {
			self.logger.Info("Redirect without location header - advancing\n")

			self.advanceToNextServer(ctx)
			return nil, errors.New("Redirect without a Location header?")
		}

		// HTTP does not allow direct redirection with
		// POST. We need to fail this request and cause an
		// immediate re-connection to the redirected URL and
		// POST the data again.

		// Since we have now learned of a new frontend, we can
		// add it to our list of URLs to try when another
		// frontend fails. It could be a new frontend that was
		// spun up after the client is created.
		found := false
		for idx, url := range self.urls {
			// Yep we already knew about it.
			if url == dest[0] {
				self.current_url_idx = idx
				found = true
			}
		}

		// No we did not know about it - add it to the end.
		if !found {
			self.urls = append(self.urls, dest[0])
			self.current_url_idx = len(self.urls) - 1
		}

		// Here self.current_url_idx points to the correct
		// frontend. Clearing the server name will force
		// rekey to that server.
		self.server_name = ""

		self.logger.Info("Redirecting to frontend: %v", dest[0])

		// Future POST requests will mark the URL as a
		// redirected URL which stops the frontend from
		// redirecting us again.
		if self.redirect_to_server <= 0 {
			self.redirect_to_server = 200

		} else {
			// For safety we wait after redirect in case we end up
			// in a redirect loop.
			wait := self.maxPoll + time.Duration(
				GetRand()(int(self.maxPollDev)))*time.Second
			self.logger.Info("Waiting after redirect: %v", wait)
			<-self.clock.After(wait)
		}

		return nil, RedirectError

	case 406:
		return nil, EnrolError

	case 200:
		encrypted := &bytes.Buffer{}

		// We need to be able to cancel the read here so we do not use
		// ioutil.ReadAll()
		n, err := utils.Copy(ctx, encrypted, resp.Body)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		self.logger.Info("%s: received %d bytes", name, n)

		// Remember the last successful index.
		self.mu.Lock()
		self.last_success_idx = self.current_url_idx
		self.mu.Unlock()

		return encrypted, nil

	default:
		self.logger.Info("Post to %v returned %v - advancing\n",
			self.GetCurrentUrl(handler), resp.StatusCode)

		// POST error - rotate to next URL
		self.advanceToNextServer(ctx)

		return nil, errors.New(resp.Status)
	}
}

// When we have any failures contacting any server, we advance our url
// index to the next frontend. When we went all the way around the
// loop we wait to backoff.  Therefore when switching from one FE to
// another we wont necessarily wait but if all frontends are down we
// wait once per loop.
func (self *HTTPConnector) advanceToNextServer(ctx context.Context) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Advance the current URL to the next one in
	// line. Reset the server name (will be fetched from
	// the PEM) and do not use redirects.
	self.current_url_idx = ((self.current_url_idx + 1) % len(self.urls))
	self.redirect_to_server = 0
	self.server_name = ""

	// We are cycling through all our frontend's PEM keys
	// in this loop. Once we go all the way around we
	// sleep to back off.
	if self.current_url_idx == self.last_success_idx {
		wait := self.maxPoll + time.Duration(
			GetRand()(int(self.maxPollDev)))*time.Second

		self.logger.Info(
			"Waiting for a reachable server: %v", wait)

		// Add random wait between polls to avoid
		// synchronization of endpoints.
		select {
		case <-self.clock.After(wait):
		case <-ctx.Done():
			return
		}
	}
}

func GetRand() func(int) int {
	mu.Lock()
	defer mu.Unlock()

	return Rand
}

func (self *HTTPConnector) String() string {
	return fmt.Sprintf("HTTP Connector to %v", self.urls)
}

// Contact the server and verify its public key. May block
// indefinitely until a valid trusted server is found. After this
// function completes the current URL is pointed at a valid server
// which should be used for all further Post() operations.  Note that
// this function holds a lock on the connector for the duration of the
// call. All other POST operations will be blocked until a valid
// server is found.
func (self *HTTPConnector) ReKeyNextServer(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		default:
			err := self.rekeyNextServer(ctx)
			if err == nil {
				return
			}

			self.advanceToNextServer(ctx)
		}
	}
}

func (self *HTTPConnector) ServerName() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.server_name
}

func (self *HTTPConnector) rekeyNextServer(ctx context.Context) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Try to fetch the server pem.
	url := self.urls[self.current_url_idx]

	req, err := http.NewRequest("GET", url+"server.pem", nil)
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("User-Agent", constants.USER_AGENT)
	req.Header.Set("Content-Type", "application/binary")

	resp, err := self.client.Do(req)
	if err != nil {
		self.logger.Info("While getting %v: %v", url, err)
		if strings.Contains(err.Error(), "cannot validate certificate") {
			self.logger.Info("If you intend to connect to a self signed " +
				"VelociraptorServer, make sure Client.use_self_signed_ssl " +
				"is set to true in the client config. If you want to use " +
				"external CAs, make sure to include all X509 root " +
				"certificates in Client.Crypto.root_certs.")
		}
		self.server_name = ""
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("Invalid status while downloading PEM")
	}

	pem, err := ioutil.ReadAll(io.LimitReader(resp.Body, constants.MAX_MEMORY))
	if err != nil {
		self.server_name = ""
		return errors.WithStack(err)
	}

	// This will replace the current server_name certificate in
	// the manager.
	server_name, err := self.manager.AddCertificate(pem)
	if err != nil {
		self.logger.Error("AddCertificate: %v", err)
		self.server_name = ""
		return err
	}

	// We must be talking to the server! The server certificate
	// must have this common name.
	if server_name != self.config_obj.Client.PinnedServerName {
		self.server_name = ""
		self.logger.Info("Invalid server certificate common name %v!", server_name)
		return errors.New("Invalid server certificate common name!")
	}

	self.server_name = server_name
	self.logger.Info("Received PEM for %v from %v", self.server_name, url)

	return nil
}

// Manages reading jobs from the reader notification channel.
type NotificationReader struct {
	config_obj *config_proto.Config
	connector  IConnector
	manager    crypto.ICryptoManager
	executor   executor.Executor
	enroller   *Enroller
	handler    string
	logger     *logging.LogContext
	name       string

	minPoll, maxPoll      time.Duration
	maxPollDev            uint64
	current_poll_duration time.Duration

	// Pause the PumpRingBufferToSendMessage loop - stops transmitting
	// data to the server temporarily. New data will still be queued
	// in the ring buffer if there is room.
	IsPaused int32

	// A callback that will be notified when the reader
	// completes. In the real client this is a fatal error since
	// without notification comms the client is unreachable. In
	// tests we ignore this.
	on_exit func()

	clock utils.Clock
}

func NewNotificationReader(
	config_obj *config_proto.Config,
	connector IConnector,
	manager crypto.ICryptoManager,
	executor executor.Executor,
	enroller *Enroller,
	logger *logging.LogContext,
	name string,
	handler string,
	on_exit func(),
	clock utils.Clock) *NotificationReader {

	maxPollDev := config_obj.Client.MaxPollStd
	if maxPollDev == 0 {
		maxPollDev = 30
	}

	return &NotificationReader{
		config_obj:            config_obj,
		connector:             connector,
		manager:               manager,
		executor:              executor,
		enroller:              enroller,
		name:                  name,
		handler:               handler,
		logger:                logger,
		minPoll:               time.Duration(1) * time.Second,
		maxPoll:               time.Duration(config_obj.Client.MaxPoll) * time.Second,
		maxPollDev:            maxPollDev,
		current_poll_duration: time.Second,
		on_exit:               on_exit,
		clock:                 clock,
	}
}

// Block until the messages are sent. Will retry, back off and rekey
// the server.
func (self *NotificationReader) sendMessageList(
	ctx context.Context, message_list [][]byte,
	urgent bool) {

	for {
		if atomic.LoadInt32(&self.IsPaused) == 0 {
			err := self.sendToURL(ctx, message_list, urgent)
			// Success!
			if err == nil {
				return
			}

			// If we are being redirected do not wait -
			// just retry again.

			if errors.Cause(err) == RedirectError {
				continue
			}

			// Failed to fetch the URL - This could happen because
			// the server is overloaded, or the client is off the
			// network. We need to back right off and retry the
			// POST again.
			self.logger.Info("Failed to fetch URL %v: %v",
				self.connector.GetCurrentUrl(self.handler), err)
		}

		// If we are paused we need to wait a bit before trying again

		// Add random wait between polls to avoid
		// synchronization of endpoints.
		wait := self.maxPoll + time.Duration(
			GetRand()(int(self.maxPollDev)))*time.Second
		self.logger.Info("Sleeping for %v", wait)

		select {
		case <-ctx.Done():
			return

		case <-self.clock.After(wait):
		}
	}

}

func (self *NotificationReader) sendToURL(
	ctx context.Context,
	message_list [][]byte,
	urgent bool) (err error) {

	if self.connector.ServerName() == "" {
		self.connector.ReKeyNextServer(ctx)
	}

	self.logger.Info("%s: Connected to %s", self.name,
		self.connector.GetCurrentUrl(self.handler))
	// Clients always compress messages to the server.
	cipher_text, err := self.manager.Encrypt(
		message_list,
		crypto_proto.PackedMessageList_ZCOMPRESSION,
		self.connector.ServerName())
	if err != nil {
		return err
	}

	encrypted, err := self.connector.Post(ctx, self.name,
		self.handler, cipher_text, urgent)

	// Enrollment is pretty quick so we need to retry sooner -
	// return no error so the next poll happens in minPoll.
	if err == EnrolError {
		if self.enroller != nil {
			self.enroller.MaybeEnrol()
		}
		return nil
	}

	if err != nil {
		return err
	}

	message_info, err := self.manager.Decrypt(encrypted.Bytes())
	if err != nil {
		return err
	}

	return message_info.IterateJobs(ctx,
		func(ctx context.Context, msg *crypto_proto.VeloMessage) {

			// Abort the client, but leave the client
			// running a bit to send acks. NOTE: This has
			// to happen before the executor gets to this
			// so we can recover the client in case the
			// executor dies.
			if msg.KillKillKill != nil {
				go func() {
					<-time.After(10 * time.Second)
					self.maybeCallOnExit()
				}()
			}

			self.executor.ProcessRequest(ctx, msg)
		})
}

func (self *NotificationReader) maybeCallOnExit() {
	if self.on_exit != nil {
		self.on_exit()
	}
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
func (self *NotificationReader) Start(
	ctx context.Context, wg *sync.WaitGroup) {

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer self.maybeCallOnExit()

		for {
			// The Reader does not send any server bound
			// messages - it is blocked reading server
			// responses.
			message_list := self.GetMessageList()
			serialized_message_list, err := proto.Marshal(message_list)
			if err == nil {
				compressed, err := utils.Compress(serialized_message_list)
				if err == nil {
					self.sendMessageList(
						ctx, [][]byte{compressed}, false)
				}
			}

			select {
			case <-ctx.Done():
				return

			case <-self.clock.After(self.minPoll):
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
	return &crypto_proto.MessageList{
		Job: []*crypto_proto.VeloMessage{{
			SessionId: constants.FOREMAN_WELL_KNOWN_FLOW,
			ForemanCheckin: &actions_proto.ForemanCheckin{
				LastEventTableVersion: actions.GlobalEventTableVersion(),
			}},
		},
	}
}

type HTTPCommunicator struct {
	config_obj *config_proto.Config

	logger *logging.LogContext

	// Read jobs from the servers notification channel.
	receiver *NotificationReader

	// Potentially enrols the client.
	enroller *Enroller

	// Sends results back to the server.
	sender *Sender

	// Will be called when we exit the communicator.
	on_exit func()
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
func (self *HTTPCommunicator) Run(
	ctx context.Context, wg *sync.WaitGroup) {
	self.logger.Info("Starting HTTPCommunicator: %v", self.receiver.connector)
	defer wg.Done()

	self.receiver.Start(ctx, wg)
	self.sender.Start(ctx, wg)

	<-ctx.Done()
}

func NewHTTPCommunicator(
	ctx context.Context,
	config_obj *config_proto.Config,
	manager crypto.IClientCryptoManager,
	executor executor.Executor,
	urls []string,
	on_exit func(),
	clock utils.Clock) (*HTTPCommunicator, error) {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	enroller := &Enroller{
		config_obj: config_obj,
		manager:    manager,
		executor:   executor,
		logger:     logger,
		clock:      clock,
	}
	connector, err := NewHTTPConnector(config_obj, manager, logger, urls, clock)
	if err != nil {
		return nil, err
	}

	rb := NewLocalBuffer(ctx, config_obj)

	// Truncate the file to ensure we always start with a clean
	// slate. This avoids a situation where the client fills up
	// the ring buffer and then is unable to send the data. When
	// it restarts it will still be unable to send the data so it
	// becomes unreachable. It is more reliable to start with a
	// clean slate each time.
	rb.Reset()

	// Make sure the buffer is reset when the program exits.
	child_on_exit := func() {
		if on_exit != nil {
			on_exit()
		}
	}

	sender, err := NewSender(
		config_obj, connector, manager, executor, rb, enroller,
		logger, "Sender", "control", child_on_exit, clock)
	if err != nil {
		return nil, err
	}

	result := &HTTPCommunicator{
		config_obj: config_obj,
		logger:     logger,
		enroller: &Enroller{
			config_obj: config_obj,
			manager:    manager,
			executor:   executor,
			logger:     logger,
			clock:      clock,
		},
		on_exit: on_exit,
		sender:  sender,
		receiver: NewNotificationReader(
			config_obj, connector, manager, executor, enroller,
			logger, "Receiver", "reader", child_on_exit, clock),
	}

	return result, nil
}

func SetProxy(handler func(*http.Request) (*url.URL, error)) {
	mu.Lock()
	defer mu.Unlock()

	proxyHandler = handler
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}
