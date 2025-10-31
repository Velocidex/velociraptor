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
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"www.velocidex.com/golang/velociraptor/utils/rand"

	"github.com/go-errors/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/crypto/storage"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

var (
	// Server sent a redirect message.
	RedirectError = errors.New("RedirectError")

	// Can be sent from connector's Post() when the server requires
	// enrolment (sending HTTP 406 status).
	EnrolError = errors.New("EnrolError")

	mu           sync.Mutex
	proxyHandler = http.ProxyFromEnvironment

	MaxRetryCount = 2
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
	next_enrollment := self.last_enrollment_time.Add(1 * time.Minute)
	now := self.clock.Now()

	// Only send an enrolment request at most every minute so as
	// not to overwhelm the server if it can not keep up.
	if now.After(next_enrollment) {
		csr_pem, err := self.manager.GetCSR()
		if err != nil {
			return
		}

		self.last_enrollment_time = utils.Now()
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
	} else {
		self.logger.Debug("Waiting for enrollment for %v",
			now.Sub(next_enrollment))
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
	nanny              *executor.NannyService

	clock utils.Clock
}

func NewHTTPConnector(
	config_obj *config_proto.Config,
	manager crypto.IClientCryptoManager,
	logger *logging.LogContext,
	urls []string,
	nanny *executor.NannyService,
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

	transport, err := networking.GetNewHttpTransport(config_obj.Client, "")
	if err != nil {
		return nil, err
	}

	transport = networking.MaybeSpyOnTransport(config_obj, transport)

	if config_obj.Client.UseSelfSignedSsl {
		logger.Info("Expecting self signed certificate for server.")

		// We only trust **our** pinned server name for HTTP comms.
		// NOTE: This stops an api cert from being presented for the
		// server. This setting also allows the server to be accessed
		// by e.g. localhost despite the certificate being issued to
		// VelociraptorServer.
		transport.TLSClientConfig.ServerName = utils.GetSuperuserName(config_obj)
	} else {
		// Not self signed - add the public roots for verifications.
		crypto.AddPublicRoots(transport.TLSClientConfig.RootCAs)
	}

	self := &HTTPConnector{
		config_obj: config_obj,
		manager:    manager,
		logger:     logger,
		clock:      clock,

		// Start with a random URL from the set of
		// preconfigured URLs. This should distribute clients
		// randomly to all frontends.
		current_url_idx: rand.Intn(len(urls)),

		minPoll:    time.Duration(1) * time.Second,
		maxPoll:    time.Duration(max_poll) * time.Second,
		maxPollDev: maxPollDev,

		urls:  urls,
		nanny: nanny,

		client: NewHTTPClient(config_obj, transport, nanny),
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

func (self *HTTPConnector) prepareRequest(
	ctx context.Context, name, handler string,
	data []byte, urgent bool) (*http.Request, error) {
	reader := bytes.NewReader(data)
	req, err := http.NewRequestWithContext(ctx,
		"POST", self.GetCurrentUrl(handler), reader)
	if err != nil {
		self.logger.Info("Post to %v returned %v - advancing to next server\n",
			self.GetCurrentUrl(handler), err)
		self.advanceToNextServer(ctx)
		return nil, errors.Wrap(err, 0)
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

	return req, nil
}

// Implement retry behavior so we can retry some errors
// immediately. This avoids having to backoff for temporary errors.
func (self *HTTPConnector) retryPost(
	ctx context.Context, name, handler string,
	data []byte, urgent bool) (resp *http.Response, err error) {

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)

	// Retry a limited number of times immediately. After this many
	// immediate tries, this function will return the error and the
	// limiter will be engaged before the next attempt.
	count := 0

	for {
		// Exit if the retry count is exceeded. Our caller will retry
		// after rate limiting.
		if count > MaxRetryCount {
			logger.Debug("%v: Exceeded retry times for %v",
				name, handler)
			if resp != nil {
				resp.Body.Close()
			}
			break
		}

		req, err := self.prepareRequest(ctx, name, handler, data, urgent)
		if err != nil {
			return nil, err
		}

		resp, err = self.client.Do(req)

		// Represents a retryable error in websockets.
		if resp != nil {
			switch resp.StatusCode {

			// 408 is infinitely retryable as it indicates the server
			// closed the connection.
			case http.StatusRequestTimeout:
				logger.Debug("%v: Retrying connection to %v: Status %v",
					name, handler, resp.StatusCode)
				if resp != nil {
					resp.Body.Close()
				}
				count++
				continue

				// 503 is retryable a couple times.
			case http.StatusServiceUnavailable:
				logger.Debug("%v: Retrying connection to %v: Status %v, %v",
					name, handler, resp.StatusCode, resp.Status)

				count++
				if resp != nil {
					resp.Body.Close()
				}
				continue
			}
		}

		// Try to connect a couple times before giving up.
		if err == notConnectedError {
			logger.Debug("%v: Retrying connection to %v: %v",
				name, handler, notConnectedError)
			count++
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}

		// No errors - we are good!
		if resp != nil && err == nil {
			return resp, err
		}

		logger.Debug("%v: Retrying connection to %v for %v time",
			name, handler, count)
		count++
		if resp != nil {
			resp.Body.Close()
		}
	}

	// Should not happen unless we messed up the logic above.
	if resp == nil && err == nil {
		err = notConnectedError
	}

	return resp, err
}

func (self *HTTPConnector) Post(
	ctx context.Context, name, handler string,
	data []byte, urgent bool) (*bytes.Buffer, error) {

	now := utils.Now()
	resp, err := self.retryPost(ctx, name, handler, data, urgent)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil && err != io.EOF {
		self.logger.Info("Post to %v returned %v - advancing to next server\n",
			self.GetCurrentUrl(handler), err)

		// POST error - rotate to next URL
		self.advanceToNextServer(ctx)
		return nil, errors.Wrap(err, 0)
	}

	self.logger.Info("%s: sent %d bytes, response with status: %v after %v, waiting for server messages",
		name, len(data), resp.StatusCode, utils.Now().Sub(now))

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
				rand.Intn(int(self.maxPollDev)))*time.Second
			self.logger.Info("Waiting after redirect: %v", wait)
			<-self.clock.After(wait)
		}

		return nil, RedirectError

		// This error means something went wrong in processing the
		// message we sent - we do not want to retry sending this
		// message because the server already attempted to process it
		// but it didnt work for some reason.
	case 400:
		data := &bytes.Buffer{}
		_, err := utils.Copy(ctx, data, resp.Body)
		if err != nil {
			return nil, errors.Wrap(err, 0)
		}

		self.logger.Error("%s: Error: %v %v", name, resp.Status, string(data.Bytes()))

		return &bytes.Buffer{}, nil

	case 406:
		return nil, EnrolError

	case 200:
		encrypted := &bytes.Buffer{}

		// We need to be able to cancel the read here.
		n, err := utils.Copy(ctx, encrypted, resp.Body)
		if err != nil {
			return nil, errors.Wrap(err, 0)
		}

		self.logger.Info("%s: received %d bytes in %v",
			name, n, utils.Now().Sub(now))

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
			rand.Intn(int(self.maxPollDev)))*time.Second

		self.logger.Info(
			"Waiting for a reachable server: %v", wait)

		// While we wait to reconnect we need to update the nanny or
		// we get killed.
		if self.nanny != nil {
			self.nanny.UpdatePumpRbToServer()
			self.nanny.UpdateReadFromServer()
		}

		// Release the lock while we wait.
		self.mu.Unlock()

		// Add random wait between polls to avoid
		// synchronization of endpoints.
		select {
		case <-self.clock.After(wait):
		case <-ctx.Done():
			return
		}

	} else {
		self.mu.Unlock()
	}
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

	// Try to get the server.pem over plain https
	if strings.HasPrefix(url, "wss://") {
		url = strings.Replace(url, "wss://", "https://", 1)
		err := self.rekeyWithURL(ctx, url)
		if err == nil {
			return nil
		}
	}

	if strings.HasPrefix(url, "ws://") {
		url = strings.Replace(url, "ws://", "http://", 1)
		err := self.rekeyWithURL(ctx, url)
		if err == nil {
			return nil
		}
	}

	return self.rekeyWithURL(ctx, url)
}

func (self *HTTPConnector) rekeyWithURL(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url+"server.pem", nil)
	if err != nil {
		return errors.Wrap(err, 0)
	}
	req.Header.Set("User-Agent", constants.USER_AGENT)
	req.Header.Set("Content-Type", "application/binary")

	resp, err := self.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

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

	if resp.StatusCode != 200 {
		err = errors.New("Invalid status while downloading PEM")
		self.logger.Info("While getting %v: %v (%d)", url, err, resp.StatusCode)
		return err
	}

	pem, err := utils.ReadAllWithLimit(resp.Body, constants.MAX_MEMORY)
	if err != nil {
		self.server_name = ""
		self.logger.Info("While reading %v: %v", url, err)
		return errors.Wrap(err, 0)
	}

	// This will replace the current server_name certificate in
	// the manager.
	server_name, err := self.manager.AddCertificate(self.config_obj, pem)
	if err != nil {
		self.logger.Error("AddCertificate: %v", err)
		self.server_name = ""
		return err
	}

	// We must be talking to the server! The server certificate
	// must have this common name.
	if server_name != utils.GetSuperuserName(self.config_obj) {
		self.server_name = ""
		self.logger.Info("Invalid server certificate common name %v!", server_name)
		return errors.New("Invalid server certificate common name!")
	}

	self.server_name = server_name
	self.logger.Info("Received PEM for %v from %v", self.server_name, url)

	storage.SetCurrentServerPem(pem)

	// Also write the server pem to the writeback if needed.
	err = writeback.GetWritebackService().MutateWriteback(self.config_obj,
		func(wb *config_proto.Writeback) error {
			server_pem := string(pem)
			if wb.LastServerPem == server_pem {
				return writeback.WritebackNoUpdate
			}

			wb.LastServerPem = server_pem
			return nil
		})

	return err
}

// Manages reading jobs from the reader notification channel.
type NotificationReader struct {
	id uint64

	// Pause the PumpRingBufferToSendMessage loop - stops transmitting
	// data to the server temporarily. New data will still be queued
	// in the ring buffer if there is room.
	IsPaused int32

	config_obj *config_proto.Config
	connector  IConnector
	manager    crypto.ICryptoManager
	executor   executor.Executor
	enroller   *Enroller

	// The url of the handler on the server (see server/comms.go)
	// Currently this is "control" for the Sender and "reader" for the
	// NotificationReader.
	handler string
	logger  *logging.LogContext
	name    string

	minPoll, maxPoll      time.Duration
	maxPollDev            uint64
	current_poll_duration time.Duration

	limiter *rate.Limiter

	// A callback that will be notified when the reader
	// completes. In the real client this is a fatal error since
	// without notification comms the client is unreachable. In
	// tests we ignore this.
	on_exit func()

	clock utils.Clock

	// Send the server Server.Internal.ClientInfo messages
	// periodically. This is sent outside the executor queues to avoid
	// having the message accumulate in the ring buffer file, but it
	// looks just like a regular montoring event query result.
	mu                 sync.Mutex
	last_update_time   time.Time
	last_update_period time.Duration

	// Cancellation can be called to restart the main loop.
	cancel func()
}

func NewNotificationReader(
	config_obj *config_proto.Config,
	connector IConnector,
	manager crypto.ICryptoManager,
	executor executor.Executor,
	enroller *Enroller,
	logger *logging.LogContext,
	name string,
	limiter *rate.Limiter,
	handler string,
	on_exit func(),
	clock utils.Clock) *NotificationReader {

	maxPollDev := config_obj.Client.MaxPollStd
	if maxPollDev == 0 {
		maxPollDev = 30
	}

	minPoll := config_obj.Client.MinPoll
	if minPoll == 0 {
		minPoll = 1
	}

	last_update_period := 86400 * time.Second
	if config_obj.Client.ClientInfoUpdateTime > 0 {
		last_update_period = time.Duration(
			config_obj.Client.ClientInfoUpdateTime) * time.Second

		// Set to a negative number to disable Server.Internal.ClientInfo
	} else if config_obj.Client.ClientInfoUpdateTime == -1 {
		last_update_period = 0
	}

	self := &NotificationReader{
		id:                    utils.GetId(),
		config_obj:            config_obj,
		connector:             connector,
		manager:               manager,
		executor:              executor,
		enroller:              enroller,
		name:                  name,
		handler:               handler,
		logger:                logger,
		minPoll:               time.Duration(minPoll) * time.Second,
		maxPoll:               time.Duration(config_obj.Client.MaxPoll) * time.Second,
		maxPollDev:            maxPollDev,
		limiter:               limiter,
		current_poll_duration: time.Second,
		on_exit:               on_exit,
		clock:                 clock,
		last_update_period:    last_update_period,
	}

	executor.Nanny().RegisterOnWarnings(self.id, func() {
		self.logger.Info("<red>%s: Nanny issued first warning!</> Restarting Receiver comms",
			self.name)
		self.Restart()
	})
	return self
}

// Block until the messages are sent. Will retry, back off and rekey
// the server.
func (self *NotificationReader) sendMessageList(
	ctx context.Context, message_list [][]byte,
	urgent bool,
	compression crypto_proto.PackedMessageList_CompressionType) {

	for {
		if atomic.LoadInt32(&self.IsPaused) == 0 {
			err := self.SendToURL(ctx, message_list, urgent, compression)
			// Success!
			if err == nil {
				return
			}

			// If we are being redirected do not wait -
			// just retry again.
			if errors.Is(err, RedirectError) {
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
			rand.Intn(int(self.maxPollDev)))*time.Second
		self.logger.Info("Sleeping for %v", wait)

		// While we wait to reconnect we need to update the nanny or
		// we get killed.
		self.executor.Nanny().UpdatePumpRbToServer()
		self.executor.Nanny().UpdateReadFromServer()

		select {
		case <-ctx.Done():
			return

		case <-self.clock.After(wait):
		}
	}

}

func (self *NotificationReader) SendToURL(
	ctx context.Context,
	message_list [][]byte,
	urgent bool,
	compression crypto_proto.PackedMessageList_CompressionType) (err error) {

	if self.connector.ServerName() == "" {
		self.connector.ReKeyNextServer(ctx)
	}

	// Clients always compress messages to the server.
	cipher_text, err := self.manager.Encrypt(
		message_list,
		compression,
		self.config_obj.Client.Nonce,
		self.connector.ServerName())
	if err != nil {
		return err
	}

	now := utils.Now()
	if !urgent {
		err := self.limiter.Wait(ctx)
		if err != nil {
			return err
		}
	}

	self.logger.Info(
		"%s: Connected to %s after waiting for limiter for %v",
		self.name, self.connector.GetCurrentUrl(self.handler),
		utils.Now().Sub(now))

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

	message_info, err := self.manager.Decrypt(ctx, encrypted.Bytes())
	if err != nil {
		return err
	}

	return message_info.IterateJobs(ctx, self.config_obj,
		func(ctx context.Context, msg *crypto_proto.VeloMessage) error {

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
			return nil
		})
}

func (self *NotificationReader) maybeCallOnExit() {
	if self.on_exit != nil {
		self.on_exit()
	}
	self.executor.Nanny().RegisterOnWarnings(self.id, nil)
}

// The Receiver channel is used to receive commands from the server:
//  1. We send an empty MessageList{} with a POST
//     (but this allows us to authenticate to the server).
//  2. Block on reading the body of the POST until the server completes
//     the request.  The server will trickle feed the connection with
//     data to keep it alive for any intermediate proxies.
//  3. Any received messages will be processed automatically by
//     self.sendMessageList()
//  4. If there are errors, we back off and wait for self.maxPoll.
func (self *NotificationReader) Start(
	ctx context.Context, wg *sync.WaitGroup) {

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer self.maybeCallOnExit()
		defer utils.CheckForPanic("Panic in main loop")

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Allow cancellation of the main loop. This will restart
			// it as needed.
			sub_ctx, cancel := context.WithCancel(ctx)
			self.cancel = cancel

			self.mainLoop(sub_ctx)
			cancel()
		}
	}()
}

func (self *NotificationReader) Restart() {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.cancel != nil {
		self.cancel()
	}
}

func (self *NotificationReader) mainLoop(ctx context.Context) {

	// Decide if we should compress the outer envelope.
	compression := crypto_proto.PackedMessageList_ZCOMPRESSION
	if self.config_obj.Client.DisableCompression {
		compression = crypto_proto.PackedMessageList_UNCOMPRESSED
	}

	// Periodically read from executor and push to ring buffer.
	for {
		self.executor.Nanny().UpdateReadFromServer()

		// The Reader does not send any server bound
		// messages - it is blocked reading server
		// responses.
		message_list := self.GetMessageList()
		serialized_message_list, err := proto.Marshal(message_list)
		if err == nil {
			if compression == crypto_proto.PackedMessageList_ZCOMPRESSION {
				compressed, err := utils.Compress(serialized_message_list)
				if err == nil {
					self.sendMessageList(
						ctx, [][]byte{compressed}, !URGENT, compression)
				}

			} else {
				self.sendMessageList(
					ctx, [][]byte{serialized_message_list}, !URGENT, compression)
			}
		}

		select {
		case <-ctx.Done():
			return

			// Reconnect quickly for low latency.
		case <-self.clock.After(self.minPoll):
			continue
		}
	}
}

// Velociraptor's foreman is very quick (since it is just an int
// comparison between the client's last hunt timestamp and the
// server's last hunt timestamp). It is therefore ok to send a foreman
// message in every reader message to improve hunt latency.
func (self *NotificationReader) GetMessageList() *crypto_proto.MessageList {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Send this every message
	result := &crypto_proto.MessageList{
		Job: []*crypto_proto.VeloMessage{{
			SessionId: constants.FOREMAN_WELL_KNOWN_FLOW,
			ForemanCheckin: &actions_proto.ForemanCheckin{
				LastEventTableVersion: self.executor.EventManager().Version(),
			},
		}}}

	// Attach the Server.Internal.ClientInfo message very
	// infrequently.
	now := utils.Now()
	if self.last_update_period > 0 &&
		now.Add(-self.last_update_period).After(self.last_update_time) {
		self.last_update_time = now

		client_info := self.executor.GetClientInfo()
		client_info_data, err := json.Marshal(client_info)
		if err == nil {
			logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
			logger.Debug("Sending client info update %v", client_info)

			client_info_data = append(client_info_data, '\n')
			result.Job = append(result.Job, &crypto_proto.VeloMessage{
				SessionId: "F.Monitoring",
				VQLResponse: &actions_proto.VQLResponse{
					JSONLResponse: string(client_info_data),
					Query: &actions_proto.VQLRequest{
						Name: "Server.Internal.ClientInfo",
					},
					TotalRows: 1,
				},
			})
		}
	}

	return result
}

type HTTPCommunicator struct {
	config_obj *config_proto.Config

	logger *logging.LogContext

	// Read jobs from the servers notification channel.
	receiver *NotificationReader

	// Potentially enrols the client.
	enroller *Enroller

	// Sends results back to the server.
	Sender *Sender

	// Will be called when we exit the communicator.
	on_exit func()

	Manager crypto.ICryptoManager
}

// Used in e2e test.
func (self *HTTPCommunicator) SetPause(is_paused bool) {
	value := int32(0)
	if is_paused {
		value = 1
	}
	atomic.StoreInt32(&self.Sender.IsPaused, value)
	atomic.StoreInt32(&self.receiver.IsPaused, value)
}

// Run forever.
func (self *HTTPCommunicator) Run(
	ctx context.Context, wg *sync.WaitGroup) {
	self.logger.Info("Starting HTTPCommunicator: %v", self.receiver.connector)

	self.receiver.Start(ctx, wg)
	self.Sender.Start(ctx, wg)

	<-ctx.Done()
}

func NewHTTPCommunicator(
	ctx context.Context,
	config_obj *config_proto.Config,
	crypto_manager crypto.IClientCryptoManager,
	executor executor.Executor,
	urls []string,
	on_exit func(),
	clock utils.Clock) (*HTTPCommunicator, error) {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	enroller := &Enroller{
		config_obj: config_obj,
		manager:    crypto_manager,
		executor:   executor,
		logger:     logger,
		clock:      clock,
	}

	// Shuffle the list of URLs so that if a server goes down,
	// clients will be distributed better accross
	// the remaining servers.
	rand.Seed(utils.Now().UnixNano())
	rand.Shuffle(len(urls), func(i, j int) {
		urls[i], urls[j] = urls[j], urls[i]
	})
	connector, err := NewHTTPConnector(
		config_obj, crypto_manager, logger, urls,
		executor.Nanny(), clock)
	if err != nil {
		return nil, err
	}

	rb := NewLocalBuffer(ctx, executor.FlowManager(), config_obj)

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
		rb.Close()
	}

	// The sender sends messages to the server. We want the sender to
	// send data as quickly as possible usually. NOTE: The sender only
	// sends data when there is something to send so we are not too
	// worried about spin loops.
	poll_min := 100 * time.Millisecond
	if config_obj.Client != nil && config_obj.Client.MinPoll > 0 {
		poll_min = time.Second * time.Duration(config_obj.Client.MinPoll)
	}

	sender_limiter := rate.NewLimiter(
		rate.Every(time.Duration(poll_min)), 100)

	sender, err := NewSender(
		config_obj, connector,
		crypto_manager, executor, rb, enroller,
		logger, "Sender", sender_limiter,

		// The handler we hit on the server to send responses.
		"control", child_on_exit, clock)
	if err != nil {
		return nil, err
	}

	// The receiver receives messages from the server.

	// We want the receiver to not poll too frequently to avoid extra
	// load on the server. Normally, the client stays connected to the
	// server so it can be tasked immediately. However sometimes if
	// the client/server connection is interrupted the client will
	// attempt to reconnect immediately but will then back off to
	// ensure it does not go into a reconnect loop. Since receiver
	// connects happen all the time we are at risk of a reeive loop -
	// where the client reconnects very frequently. This limiter
	// avoids this condition by rate limiting the frequency of reader
	// connections.
	poll_max := 60 * time.Second
	if config_obj.Client != nil && config_obj.Client.MaxPoll > 0 {
		poll_max = time.Second * time.Duration(config_obj.Client.MaxPoll)
	}

	// In the case of a reconnect loop we do not connect more than
	// twice every poll max but we are allowed to connect sooner at
	// first. Note: We set the limit to half the max poll rate because
	// the client connects at least as frequently as the max poll
	// rate. We need to set the limit lower to allow the limiter to
	// gain tokens during normal operation.
	receiver_limiter := rate.NewLimiter(
		rate.Every(time.Duration(poll_max/2)), 10)

	receiver := NewNotificationReader(
		config_obj, connector, crypto_manager, executor, enroller,
		logger, "Receiver "+executor.ClientId(), receiver_limiter,

		// The handler for receiving messages from the server.
		"reader", child_on_exit, clock)

	result := &HTTPCommunicator{
		config_obj: config_obj,
		logger:     logger,
		enroller: &Enroller{
			config_obj: config_obj,
			manager:    crypto_manager,
			executor:   executor,
			logger:     logger,
			clock:      clock,
		},
		on_exit:  on_exit,
		Sender:   sender,
		receiver: receiver,
		Manager:  crypto_manager,
	}

	return result, nil
}

func SetProxy(handler func(*http.Request) (*url.URL, error)) {
	mu.Lock()
	defer mu.Unlock()

	proxyHandler = handler
}

func GetProxy() func(*http.Request) (*url.URL, error) {
	mu.Lock()
	defer mu.Unlock()

	return proxyHandler
}

func init() {
	rand.Seed(utils.Now().UTC().UnixNano())
}
