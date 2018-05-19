package responder

import (
	"github.com/golang/protobuf/proto"
	"reflect"
	"runtime/debug"
	"strings"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type Responder struct {
	request *crypto_proto.GrrMessage
	next_id uint64
	output  chan<- *crypto_proto.GrrMessage
}

// NewResponder returns a new Responder.
func NewResponder(
	request *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) *Responder {
	result := &Responder{
		request: request,
		next_id: 0,
		output:  output,
	}
	return result
}

func (self *Responder) AddResponse(message proto.Message) error {
	components := strings.Split(proto.MessageName(message), ".")
	rdf_name := components[len(components)-1]
	self.next_id = self.next_id + 1
	response := &crypto_proto.GrrMessage{
		SessionId:   self.request.SessionId,
		RequestId:   self.request.RequestId,
		ResponseId:  proto.Uint64(self.next_id),
		ArgsRdfName: &rdf_name,
	}

	serialized_args, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	response.Args = serialized_args

	if rdf_name == "GrrStatus" {
		response.Type = crypto_proto.GrrMessage_STATUS.Enum()
	}

	self.output <- response

	return nil
}

func (self *Responder) RaiseError(message string) {
	backtrace := string(debug.Stack())
	status := &crypto_proto.GrrStatus{
		Backtrace:    &backtrace,
		ErrorMessage: &message,
		Status:       crypto_proto.GrrStatus_GENERIC_ERROR.Enum(),
	}
	self.AddResponse(status)
}

func (self *Responder) Return() {
	status := &crypto_proto.GrrStatus{
		Status: crypto_proto.GrrStatus_OK.Enum(),
	}
	self.AddResponse(status)
}

func (self *Responder) SendResponseToWellKnownFlow(
	flow_name string, message proto.Message) error {
	components := strings.Split(proto.MessageName(message), ".")
	rdf_name := components[len(components)-1]
	response := &crypto_proto.GrrMessage{
		SessionId:   &flow_name,
		ResponseId:  proto.Uint64(1),
		ArgsRdfName: &rdf_name,
	}

	serialized_args, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	response.Args = serialized_args
	self.output <- response
	return nil
}

func (self *Responder) GetArgs() proto.Message {
	return ExtractGrrMessagePayload(self.request)
}

func (self *Responder) SessionId() string {
	return *self.request.SessionId
}

// Unpack the GrrMessage payload. The return value should be type asserted.
func ExtractGrrMessagePayload(message *crypto_proto.GrrMessage) proto.Message {
	message_type := proto.MessageType("proto." + *message.ArgsRdfName)
	if message_type != nil {
		new_message := reflect.New(message_type.Elem()).Interface().(proto.Message)
		err := proto.Unmarshal(message.Args, new_message)
		if err != nil {
			return nil
		}
		return new_message
	}
	return nil
}

func NewRequest(message proto.Message) (*crypto_proto.GrrMessage, error) {
	components := strings.Split(proto.MessageName(message), ".")
	rdf_name := components[len(components)-1]
	response := &crypto_proto.GrrMessage{
		SessionId:   proto.String("XYZ"),
		RequestId:   proto.Uint64(1),
		ArgsRdfName: &rdf_name,
	}

	serialized_args, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	response.Args = serialized_args

	return response, nil
}
