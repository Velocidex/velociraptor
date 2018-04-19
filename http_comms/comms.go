package http_comms

import (
	"github.com/golang/protobuf/proto"
	"errors"
	"sync"
	"bytes"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
	"www.velocidex.com/golang/velociraptor/context"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
//	utils "www.velocidex.com/golang/velociraptor/testing"
)

type HTTPCommunicator struct {
	ctx *context.Context
	urls             []string
	minPoll, maxPoll int
	client           *http.Client
	executor          executor.Executor
	manager          *crypto.CryptoManager
	server_name string

	// mutex guards pending_message_list.
	mutex *sync.Mutex
	pending_message_list *crypto_proto.MessageList

	last_ping_time time.Time
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
				return
			}
			self.mutex.Lock()
			self.pending_message_list.Job = append(
				self.pending_message_list.Job, msg)
			self.mutex.Unlock()
		}
	} ()


	// Check the pending message list for messages every poll_min.
	for {
		self.mutex.Lock()
		// Blocks executor while we transfer this data. This
		// avoids overrun of internal queue for slow networks.
		if len(self.pending_message_list.Job) > 0 {
			// There is nothing we can do in case of
			// failure here except just keep trying until
			// we have to drop the packets on the floor.
			self.sendMessageList(self.pending_message_list)

			// Clear the pending_message_list for next time.
			self.pending_message_list = &crypto_proto.MessageList{}
		}
		self.mutex.Unlock()

		// We are due for an unsolicited poll.
		if time.Now().After(
			self.last_ping_time.Add(self.current_poll_duration)) {
				self.mutex.Lock()
				log.Printf("Sending unsolicited ping.")
				self.current_poll_duration *= 2

				self.sendMessageList(self.pending_message_list)
				self.pending_message_list = &crypto_proto.MessageList{}
				self.mutex.Unlock()
			}

		select {
		case <- self.ctx.Done():
			log.Printf("Stopping HTTPCommunicator")
			return

		case <- time.After(1 * time.Second):
			continue
		}
	}
}

// Keep trying to send the messages forever.
func (self *HTTPCommunicator) sendMessageList(message_list *crypto_proto.MessageList) {
	self.last_ping_time = time.Now()

	for {
		for _, url := range self.urls {
			err := self.sendToURL(url, message_list)
			if err != nil {
				log.Printf("Failed to fetch URL %v: %v", url, err)

				select {
				case <- self.ctx.Done():
					return
				case <- time.After(1 * time.Second):
					continue
				}

			} else {
				return
			}
		}
	}

}

func (self *HTTPCommunicator) sendToURL(
	url string,
	message_list *crypto_proto.MessageList) error {
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
	server_name, err := self.manager.AddCertificate(pem)
	if err != nil {
		return err
	}
	self.server_name = *server_name
	log.Printf("Received PEM for: %v", self.server_name)

	// We are now ready to communicate with the server.
	cipher_text, err := self.manager.EncryptMessageList(
		message_list, self.server_name)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(cipher_text)
	resp, err = self.client.Post(
		url + "control?api=3", "application/binary", reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Printf("Received response with status: %v", resp.Status)
	if resp.StatusCode == 406 {
		self.MaybeEnrol()
		return nil
	}

	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}

	encrypted, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	response_message_list, err := self.manager.DecryptMessageList(encrypted)
	if err != nil {
		return err
	}

	// Feed the messages to the executor.
	if len(response_message_list.Job) > 0 {
		// The server sent some messages, so we need to switch
		// to fast poll mode.
		self.current_poll_duration = 1 * time.Second

		for _, msg := range response_message_list.Job {
			self.executor.ProcessRequest(msg)
		}
	}

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
				Type: crypto_proto.Certificate_CSR.Enum(),
				Pem: csr_pem,
			}

			arg_rdf_name := "Certificate"
			reply := &crypto_proto.GrrMessage{
				SessionId: &constants.ENROLLMENT_WELL_KNOWN_FLOW,
				ArgsRdfName: &arg_rdf_name,
				Priority: crypto_proto.GrrMessage_HIGH_PRIORITY.Enum(),
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
	result := &HTTPCommunicator{minPoll: 1, maxPoll: 10, urls: urls, ctx: &ctx}
	result.mutex = &sync.Mutex{}
	result.pending_message_list = &crypto_proto.MessageList{}
	result.executor = executor
	result.current_poll_duration = 1 * time.Second
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
