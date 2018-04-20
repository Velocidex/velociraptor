package executor

import (
	"log"
	"fmt"
	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
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
	ctx *context.Context
	Inbound  chan *crypto_proto.GrrMessage
	Outbound chan *crypto_proto.GrrMessage
	plugins map[string]actions.ClientAction
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
	msg := <-self.Outbound
	return msg
}

func makeUnknownActionResponse(req *crypto_proto.GrrMessage) *crypto_proto.GrrMessage {
	var response uint64 = 1
	reply := &crypto_proto.GrrMessage{
		SessionId:  req.SessionId,
		RequestId:  req.RequestId,
		ResponseId: &response,
		Type:       crypto_proto.GrrMessage_STATUS.Enum(),
	}
	error_message := fmt.Sprintf("Client action '%v' not known", *req.Name)
	status := &crypto_proto.GrrStatus{
		Status: crypto_proto.GrrStatus_GENERIC_ERROR.Enum(),
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
	req *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {

	// Never serve unauthenticated requests.
	if *req.AuthState != *crypto_proto.GrrMessage_AUTHENTICATED.Enum() {
		log.Printf("Unauthenticated")
		utils.Debug(req.AuthState)
		return []*crypto_proto.GrrMessage{
			makeUnknownActionResponse(req),
		}
	}

	plugin, pres := self.plugins[*req.Name]
	if !pres {
		return []*crypto_proto.GrrMessage{
			makeUnknownActionResponse(req),
		}
	}

	return plugin.Run(self.ctx, req)
}


func NewClientExecutor(ctx *context.Context) (*ClientExecutor, error) {
	result := &ClientExecutor{}
	result.ctx = ctx
	result.Inbound = make(chan *crypto_proto.GrrMessage)
	result.Outbound = make(chan *crypto_proto.GrrMessage)
	result.plugins = actions.GetClientActionsMap()
	go func() {
		for {
			// Pump messages from input channel and just
			// fail them on the output channel.
			req := result.ReadFromServer()
			utils.Debug(req)

			go func() {
				for _, msg := range result.processRequestPlugin(req) {
					result.SendToServer(msg)
				}
			}()
		}
	}()

	return result, nil
}
