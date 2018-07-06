package http_comms

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/context"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
)

type HTTPCommunicator struct {
	ctx              *context.Context
	current_url_idx  int
	urls             []string
	minPoll, maxPoll time.Duration
	client           *http.Client
	executor         executor.Executor

	// The Crypto Manager for communicating with the current
	// URL. Note, when the URL is changed, the CryptoManager is
	// replaced. The CryptoManager is initialized by a successful
	// connection to the URL's server.pem endpoint.
	manager *crypto.CryptoManager

	// The current server name.
	server_name string

	pending_messages chan *crypto_proto.GrrMessage

	last_ping_time        time.Time
	current_poll_duration time.Duration

	// Enrollment
	last_enrollment_time time.Time
}

// Run forever.
func (self *HTTPCommunicator) Run() {
	log.Printf("Starting HTTPCommunicator: %v", self.urls)

	// Pump messages from the executor to the pending message list.
	go func() {
		for {
			msg := self.executor.ReadResponse()
			// Executor closed the channel.
			if msg == nil {
				close(self.pending_messages)
				return
			}

			self.pending_messages <- msg
		}
	}()

	// Check the pending message list for messages every poll_min.
	// A note about timing: This loops is quantized to
	// self.minPoll which means that polls can never occur more
	// frequently than that. The minPoll duration allows the
	// client enough time to queue up several messages in the same
	// POST operation. When there is nothing to send, the poll
	// interval will grow gradually to maxPoll.

	// If an error occurs, the client will retry at maxPoll until
	// the URL is successful. If there is data to send the client
	// will switch to fast poll mode until there is no more data
	// to send, then it will back off.
	for {
		// If there is some data in the queues we send it
		// immediately.
		message_list := self.drainMessageQueue()
		if len(message_list.Job) > 0 {
			self.sendMessageList(message_list)

			// We are due for an unsolicited poll.
		} else if time.Now().After(
			self.last_ping_time.Add(self.current_poll_duration)) {
			log.Printf("Sending unsolicited ping.")
			self.current_poll_duration *= 2
			if self.current_poll_duration > self.maxPoll {
				self.current_poll_duration = self.maxPoll
			}

			self.sendMessageList(message_list)
		}

		// Sleep for minPoll
		select {
		case <-self.ctx.Done():
			log.Printf("Stopping HTTPCommunicator")
			return

		case <-time.After(self.minPoll):
			continue
		}
	}
}

// Pull off as many messages as we can off the channel to send. Note:
// As we drain the channel the executor will be woken to fill it up
// again - since the self.pending_messages channel has a buffer.
func (self *HTTPCommunicator) drainMessageQueue() *crypto_proto.MessageList {
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

	return result
}

func (self *HTTPCommunicator) sendMessageList(message_list *crypto_proto.MessageList) {
	self.last_ping_time = time.Now()

	for {
		url := self.urls[self.current_url_idx]
		err := self.sendToURL(url, message_list)
		// Success!
		if err == nil {
			return
		}

		log.Printf("Failed to fetch URL %v: %v", url, err)

		select {
		case <-self.ctx.Done():
			return

			// Wait for the maximum length of time
			// and try the next URL.
		case <-time.After(self.maxPoll):
			self.server_name = ""
			self.current_url_idx = ((self.current_url_idx + 1) % len(self.urls))

			continue
		}
	}

}

func (self *HTTPCommunicator) sendToURL(
	url string,
	message_list *crypto_proto.MessageList) error {

	if self.server_name == "" {
		// Try to fetch the server pem.
		resp, err := self.client.Get(url + "server.pem")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		pem, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		// TODO: Verify the server pem.
		// This will replace the current server_name
		// certificate in the manager.
		server_name, err := self.manager.AddCertificate(pem)
		if err != nil {
			return err
		}
		self.server_name = *server_name
		log.Printf("Received PEM for %v from %v", self.server_name, url)
	}

	// We are now ready to communicate with the server.
	cipher_text, err := self.manager.EncryptMessageList(
		message_list, self.server_name)
	if err != nil {
		return err
	}

	reader := bytes.NewReader(cipher_text)
	resp, err := self.client.Post(
		url+"control?api=3", "application/binary", reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Printf("Received response with status: %v", resp.Status)
	// 406 status means we need to enrol since the server is
	// unable to talk to us because it does not have our public
	// key.
	if resp.StatusCode == 406 {
		self.MaybeEnrol()
		return nil
	}

	// Other errors will be propagated and retried.
	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}

	// Success! Decrypt the messages and pump them into the
	// executor.
	encrypted, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	response_message_list, err := self.manager.DecryptMessageList(encrypted)
	if err != nil {
		return err
	}

	// Note: We queue requests to the executor in a go routine
	// because the executor is typically blocked during the entire
	// HTTP transaction.
	go func() {
		// Feed the messages to the executor.
		if len(response_message_list.Job) > 0 {
			// The server sent some messages, so we need to switch
			// to fast poll mode.
			self.current_poll_duration = self.minPoll

			for _, msg := range response_message_list.Job {
				self.executor.ProcessRequest(msg)
			}
		}
	}()

	return nil
}

func (self *HTTPCommunicator) MaybeEnrol() {
	// Only send an enrolment request at most every 10 minutes so
	// as not to overwhelm the server if it can not keep up.
	if time.Now().After(
		self.last_enrollment_time.Add(10 * time.Minute)) {
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
		log.Printf("Enrolling")
		go func() {
			self.executor.SendToServer(reply)
		}()
	}
}

func NewHTTPCommunicator(
	ctx context.Context,
	manager *crypto.CryptoManager,
	executor executor.Executor,
	urls []string) (*HTTPCommunicator, error) {
	result := &HTTPCommunicator{
		minPoll: time.Duration(1) * time.Second,
		maxPoll: time.Duration(10) * time.Second,
		urls:    urls,
		ctx:     &ctx}

	// Allow the executor to queue 100 messages in the same packet.
	result.pending_messages = make(chan *crypto_proto.GrrMessage, 100)
	result.executor = executor
	result.current_poll_duration = result.minPoll
	result.client = &http.Client{
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
	}

	result.manager = manager

	return result, nil
}
