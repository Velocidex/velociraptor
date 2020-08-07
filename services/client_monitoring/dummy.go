package client_monitoring

import (
	"errors"

	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

type Dummy struct{}

func (self Dummy) CheckClientEventsVersion(client_id string, client_version uint64) bool {
	return false
}

func (self Dummy) GetClientUpdateEventTableMessage(client_id string) *crypto_proto.GrrMessage {
	return &crypto_proto.GrrMessage{}
}

func (self Dummy) GetClientMonitoringState() *flows_proto.ClientEventTable {
	return &flows_proto.ClientEventTable{}
}

func (self Dummy) SetClientMonitoringState(state *flows_proto.ClientEventTable) error {
	return errors.New("No valie client monitoring service running")
}

func init() {
	services.RegisterClientEventManager(Dummy{})
}
