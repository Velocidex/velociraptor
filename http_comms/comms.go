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
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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
	config_obj           *api_proto.Config
	manager              crypto.ICryptoManager
	executor             executor.Executor
	logger               *logging.LogContext
	last_enrollment_time time.Time
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
	result := &crypto_proto.MessageList{}

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
		return result
	}

	reply.Args = serialized_arg
	result.Job = append(result.Job, reply)

	return result
}

type IConnector interface {
	GetCurrentUrl() string
	Post(handler string, data []byte) (*http.Response, error)
	ReKeyNextServer()
	ServerName() string
}

// Responsible for using HTTP to talk with the end point.
type HTTPConnector struct {
	// The Crypto Manager for communicating with the current
	// URL. Note, when the URL is changed, the CryptoManager is
	// initialized by a successful connection to the URL's
	// server.pem endpoint.
	manager crypto.ICryptoManager
	logger  *logging.LogContext

	minPoll, maxPoll time.Duration
	maxPollDev       uint64
	// Used to cycle through the urls slice.
	mu              sync.Mutex
	current_url_idx int
	urls            []string

	client *http.Client

	// Obtained from the server's Cert CommonName.
	server_name string
}

func NewHTTPConnector(
	config_obj *api_proto.Config,
	manager crypto.ICryptoManager,
	logger *logging.LogContext) *HTTPConnector {

	max_poll := config_obj.Client.MaxPoll
	if max_poll == 0 {
		max_poll = 60
	}

	maxPollDev := config_obj.Client.MaxPollStd
	if maxPollDev == 0 {
		maxPollDev = 30
	}

	tls_config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// For self signed certificates we must ignore the server name
	// and only trust certs issued by our server.
	if config_obj.Client.UseSelfSignedSsl {
		logger.Info("Expecting self signed certificate for server.")

		CA_Pool := x509.NewCertPool()
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))

		tls_config.ServerName = constants.FRONTEND_NAME

		// We only trust **our** root CA.
		tls_config.RootCAs = CA_Pool
	}

	return &HTTPConnector{
		manager: manager,
		logger:  logger,

		minPoll:    time.Duration(1) * time.Second,
		maxPoll:    time.Duration(max_poll) * time.Second,
		maxPollDev: maxPollDev,

		urls: config_obj.Client.ServerUrls,

		client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   300 * time.Second,
					KeepAlive: 300 * time.Second,
					DualStack: true,
				}).DialContext,
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          100,
				IdleConnTimeout:       300 * time.Second,
				TLSHandshakeTimeout:   100 * time.Second,
				ExpectContinueTimeout: 10 * time.Second,
				ResponseHeaderTimeout: 100 * time.Second,
				TLSClientConfig:       tls_config,
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

func (self *HTTPConnector) String() string {
	return fmt.Sprintf("HTTP Connector to %v", self.urls)
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

		// Only wait once we go round the list a full time.
		if self.current_url_idx == 0 {
			wait := self.maxPoll + time.Duration(
				rand.Intn(int(self.maxPollDev)))*time.Second

			self.logger.Info(
				"Waiting for a reachable server: %v", wait)

			// Add random wait between polls to avoid
			// synchronization of endpoints.
			<-time.After(wait)
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
	if self.server_name == "" {
		self.current_url_idx = ((self.current_url_idx + 1) % len(self.urls))
	}

	// Try to fetch the server pem.
	url := self.urls[self.current_url_idx]
	resp, err := self.client.Get(url + "server.pem")
	if err != nil {
		self.logger.Info("While getting %v: %v", url, err)
		return err
	}
	defer resp.Body.Close()

	pem, err := ioutil.ReadAll(io.LimitReader(resp.Body, constants.MAX_MEMORY))
	if err != nil {
		return errors.WithStack(err)
	}

	// This will replace the current server_name
	// certificate in the manager.
	server_name, err := self.manager.AddCertificate(pem)
	if err != nil {
		self.logger.Error(err)
		return err
	}

	// We must be talking to the server! The server certificate
	// must have this common name.
	if *server_name != constants.FRONTEND_NAME {
		self.logger.Info("Invalid server certificate common name %v!", *server_name)
		return errors.New("Invalid server certificate common name!")
	}

	self.server_name = *server_name
	self.logger.Info("Received PEM for %v from %v", self.server_name, url)

	return nil
}

// Manages reading jobs from the reader notification channel.
type NotificationReader struct {
	config_obj api_proto.Config
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
	IsPaused              int32
}

func NewNotificationReader(
	config_obj *api_proto.Config,
	connector IConnector,
	manager crypto.ICryptoManager,
	executor executor.Executor,
	enroller *Enroller,
	logger *logging.LogContext,
	name string,
	handler string) *NotificationReader {

	maxPollDev := config_obj.Client.MaxPollStd
	if maxPollDev == 0 {
		maxPollDev = 30
	}

	return &NotificationReader{
		config_obj:            *config_obj,
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
	}
}

// Block until the messages are sent. Will retry, back off and rekey
// the server.
func (self *NotificationReader) sendMessageList(
	ctx context.Context, message_list []byte) {

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

		// Add random wait between polls to avoid
		// synchronization of endpoints.
		wait := self.maxPoll + time.Duration(
			rand.Intn(int(self.maxPollDev)))*time.Second

		select {
		case <-ctx.Done():
			return

			// Wait for the maximum length of time
			// and try to rekey the next URL.
		case <-time.After(wait):
			self.connector.ReKeyNextServer()
			continue
		}
	}
}

func (self *NotificationReader) sendToURL(
	ctx context.Context, message_list []byte) error {

	if self.connector.ServerName() == "" {
		self.connector.ReKeyNextServer()
	}

	self.logger.Info("%s: Connected to %s", self.name,
		self.connector.GetCurrentUrl()+self.handler)

	cipher_text, err := self.manager.Encrypt(
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
		if self.enroller != nil {
			self.enroller.MaybeEnrol()
		}
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
				fmt.Printf("Error: %v\n", err)
				return errors.WithStack(err)
			}
			if n == 0 {
				break process_response
			}

			encrypted = append(encrypted, buf[:n]...)
			if len(encrypted) > int(self.config_obj.Client.MaxUploadSize) {
				return errors.New("Response too long")
			}
		}
	}

	response_message_list, err := crypto.DecryptMessageList(
		self.manager, encrypted)
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
			message_list := self.GetMessageList()
			serialized_message_list, err := proto.Marshal(message_list)
			if err == nil {
				self.sendMessageList(ctx, serialized_message_list)
			}

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
	result := &crypto_proto.MessageList{}
	reply := &crypto_proto.GrrMessage{
		SessionId:   constants.FOREMAN_WELL_KNOWN_FLOW,
		ArgsRdfName: "ForemanCheckin",
		Priority:    crypto_proto.GrrMessage_LOW_PRIORITY,
		ClientType:  crypto_proto.GrrMessage_VELOCIRAPTOR,
	}

	serialized_arg, err := proto.Marshal(&actions_proto.ForemanCheckin{
		LastHuntTimestamp:     self.config_obj.Writeback.HuntLastTimestamp,
		LastEventTableVersion: actions.GlobalEventTableVersion(),
	})
	if err != nil {
		return result
	}

	reply.Args = serialized_arg
	result.Job = append(result.Job, reply)

	return result
}

type HTTPCommunicator struct {
	config_obj *api_proto.Config

	logger *logging.LogContext

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
	self.logger.Info("Starting HTTPCommunicator: %v", self.receiver.connector)

	self.receiver.Start(ctx)
	self.sender.Start(ctx)

	<-ctx.Done()
}

func NewHTTPCommunicator(
	config_obj *api_proto.Config,
	manager crypto.ICryptoManager,
	executor executor.Executor,
	urls []string) (*HTTPCommunicator, error) {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	enroller := &Enroller{
		config_obj: config_obj,
		manager:    manager,
		executor:   executor,
		logger:     logger}
	connector := NewHTTPConnector(config_obj, manager, logger)

	rb := NewLocalBuffer(config_obj)

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
			config_obj, connector, manager, executor, rb, enroller,
			logger, "Sender", "control"),
		receiver: NewNotificationReader(
			config_obj, connector, manager, executor, enroller,
			logger, "Receiver", "reader"),
	}

	return result, nil
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}
