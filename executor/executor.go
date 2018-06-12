package executor

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"log"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

type Executor interface {
	// These are called by the executor code.
	ReadFromServer() *crypto_proto.GrrMessage
	SendToServer(message *crypto_proto.GrrMessage)

	// These two are called by the comms module.

	// Feed a server request to the executor for execution.
	ProcessRequest(message *crypto_proto.GrrMessage)

	// Read a single response from the executor to be sent to the server.
	ReadResponse() *crypto_proto.GrrMessage
}

// A concerete implementation of a client executor.

type ClientExecutor struct {
	Inbound  chan *crypto_proto.GrrMessage
	Outbound chan *crypto_proto.GrrMessage
	plugins  map[string]actions.ClientAction
}

// Blocks until a request is received from the server. Called by the
// Executors internal processor.
func (self *ClientExecutor) ReadFromServer() *crypto_proto.GrrMessage {
	msg := <-self.Inbound
	return msg
}

func (self *ClientExecutor) SendToServer(message *crypto_proto.GrrMessage) {
	self.Outbound <- message
}

func (self *ClientExecutor) ProcessRequest(message *crypto_proto.GrrMessage) {
	self.Inbound <- message
}

func (self *ClientExecutor) ReadResponse() *crypto_proto.GrrMessage {
	if msg, ok := <-self.Outbound; ok {
		return msg
	}
	return nil
}

func makeUnknownActionResponse(req *crypto_proto.GrrMessage) *crypto_proto.GrrMessage {
	var response uint64 = 1
	reply := &crypto_proto.GrrMessage{
		SessionId:  req.SessionId,
		RequestId:  req.RequestId,
		ResponseId: &response,
		Type:       crypto_proto.GrrMessage_STATUS.Enum(),
		ClientType: crypto_proto.GrrMessage_VELOCIRAPTOR.Enum(),
	}

	if req.TaskId != nil {
		reply.TaskId = req.TaskId
	}

	error_message := fmt.Sprintf("Client action '%v' not known", *req.Name)
	status := &crypto_proto.GrrStatus{
		Status:       crypto_proto.GrrStatus_GENERIC_ERROR.Enum(),
		ErrorMessage: &error_message,
	}

	status_marshalled, err := proto.Marshal(status)
	args_name := "GrrStatus"
	if err == nil {
		reply.Args = status_marshalled
		reply.ArgsRdfName = &args_name
	}

	return reply
}

func (self *ClientExecutor) processRequestPlugin(
	ctx *context.Context,
	req *crypto_proto.GrrMessage) {

	// Never serve unauthenticated requests.
	if req.AuthState != nil &&
		*req.AuthState != *crypto_proto.GrrMessage_AUTHENTICATED.Enum() {
		log.Printf("Unauthenticated")
		self.SendToServer(makeUnknownActionResponse(req))
		return
	}

	if req.Name == nil {
		return
	}

	plugin, pres := self.plugins[*req.Name]
	if !pres {
		self.SendToServer(makeUnknownActionResponse(req))
		return
	}

	receive_chan := make(chan *crypto_proto.GrrMessage)

	// Run the plugin in the other thread and drain its messages
	// to send to the server.
	go func() {
		plugin.Run(ctx, req, receive_chan)
		close(receive_chan)
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-receive_chan:
			if !ok {
				return
			}
			self.SendToServer(msg)
		}
	}
}

func NewClientExecutor(config_obj *config.Config) (*ClientExecutor, error) {
	result := &ClientExecutor{}
	result.Inbound = make(chan *crypto_proto.GrrMessage)
	result.Outbound = make(chan *crypto_proto.GrrMessage)
	result.plugins = actions.GetClientActionsMap()
	go func() {
		for {
			// Pump messages from input channel and just
			// fail them on the output channel.
			req := result.ReadFromServer()

			// Ignore unauthenticated messages - the
			// server should never send us those.
			if *req.AuthState == crypto_proto.GrrMessage_AUTHENTICATED {
				// Each request has its own context.
				ctx := context.BackgroundFromConfig(config_obj)
				utils.Debug(req)
				result.processRequestPlugin(ctx, req)
			}
		}
	}()

	return result, nil
}
