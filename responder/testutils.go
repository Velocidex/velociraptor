package responder

import crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"

func TestResponder() *Responder {
	return &Responder{
		output:  make(chan *crypto_proto.GrrMessage, 100),
		request: &crypto_proto.GrrMessage{},
	}
}

func GetTestResponses(self *Responder) []*crypto_proto.GrrMessage {
	close(self.output)
	result := []*crypto_proto.GrrMessage{}
	for item := range self.output {
		result = append(result, item)
	}

	return result
}
