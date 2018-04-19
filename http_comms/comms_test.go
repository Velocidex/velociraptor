package http_comms

import (
	//	"github.com/stretchr/testify/assert"
	//	"github.com/stretchr/testify/suite"
	"testing"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/context"
	"www.velocidex.com/golang/velociraptor/crypto"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	utils "www.velocidex.com/golang/velociraptor/testing"
	"github.com/golang/protobuf/proto"
)


type TestExecutor struct {
	Inbound          chan *crypto_proto.GrrMessage
	Outbound         chan *crypto_proto.GrrMessage
}

// Blocks until a request is received from the server. Called by the
// Executors internal processor.
func (self *TestExecutor) ReadFromServer() *crypto_proto.GrrMessage {
	msg := <- self.Inbound
	return msg
}

func (self *TestExecutor) SendToServer(message *crypto_proto.GrrMessage) {
	self.Outbound <- message
}

func (self *TestExecutor) ProcessRequest(message *crypto_proto.GrrMessage) {
	self.Inbound <- message
}

// Blocks until
func (self *TestExecutor) ReadResponse() *crypto_proto.GrrMessage {
	msg := <- self.Outbound
	return msg
}


func NewTestExecutor(t *testing.T) *TestExecutor {
	result := &TestExecutor{}
	result.Inbound = make(chan *crypto_proto.GrrMessage)
	result.Outbound = make(chan *crypto_proto.GrrMessage)

	go func () {
		for {
			// Pump messages from input channel and just
			// fail them on the output channel.
			msg := result.ReadFromServer()
			utils.Debug(msg)

			var response uint64 = 1
			reply := &crypto_proto.GrrMessage{
				SessionId: msg.SessionId,
				RequestId: msg.RequestId,
				ResponseId: &response,
				Type: crypto_proto.GrrMessage_STATUS.Enum(),
			}
			status := &crypto_proto.GrrStatus{
				Status: crypto_proto.GrrStatus_GENERIC_ERROR.Enum(),
			}

			status_marshalled, err := proto.Marshal(status)
			if err != nil {
				t.Fatal(err)
			}

			copy(reply.Args, status_marshalled)
			go func() {
				result.SendToServer(reply)
			}()
		}
	}()

	return result
}


func (self TestExecutor) GetInbound() chan *crypto_proto.GrrMessage {
	return self.Inbound
}

func (self TestExecutor) GetOutbound() chan *crypto_proto.GrrMessage {
	return self.Outbound
}



func TestHTTPComms(t *testing.T) {
	ctx := context.Background()
	utils.Debug(ctx)

	config, err := config.LoadConfig("test_data/client.config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	manager, err := crypto.NewClientCryptoManager(
		&ctx, []byte(config.Client_private_key))
	if err != nil {
		t.Fatal(err)
	}

	exe := NewTestExecutor(t)

	comm, err := NewHTTPCommunicator(
		ctx,
		manager,
		exe,
		[]string{
			"http://localhost:8080/",
		})
	if err != nil {
		t.Fatal(err)
	}

	comm.Run()
}
