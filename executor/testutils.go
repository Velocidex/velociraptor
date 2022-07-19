package executor

import (
	"context"

	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type TestExecutor struct{}

func (self *TestExecutor) ClientId() string {
	return ""
}

func (self *TestExecutor) ReadFromServer() *crypto_proto.VeloMessage {
	return nil
}
func (self *TestExecutor) SendToServer(message *crypto_proto.VeloMessage) {}
func (self *TestExecutor) ProcessRequest(
	ctx context.Context,
	message *crypto_proto.VeloMessage) {
}
func (self *TestExecutor) ReadResponse() <-chan *crypto_proto.VeloMessage {
	return nil
}
