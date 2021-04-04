package responder

import crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"

func TestResponder() *Responder {
	return &Responder{
		output:  make(chan *crypto_proto.VeloMessage, 100),
		request: &crypto_proto.VeloMessage{},
	}
}

func GetTestResponses(self *Responder) []*crypto_proto.VeloMessage {
	close(self.output)
	result := []*crypto_proto.VeloMessage{}
	for item := range self.output {
		result = append(result, item)
	}

	return result
}
