package client_monitoring

import (
	"context"
	"errors"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

type Dummy struct{}

func (self Dummy) CheckClientEventsVersion(
	config_obj *config_proto.Config,
	client_id string, client_version uint64) bool {
	return false
}

func (self Dummy) GetClientUpdateEventTableMessage(
	config_obj *config_proto.Config,
	client_id string) *crypto_proto.GrrMessage {
	return &crypto_proto.GrrMessage{}
}

func (self Dummy) GetClientMonitoringState() *flows_proto.ClientEventTable {
	return &flows_proto.ClientEventTable{}
}

func (self Dummy) SetClientMonitoringState(
	ctx context.Context,
	config_obj *config_proto.Config,
	state *flows_proto.ClientEventTable) error {
	return errors.New("No valie client monitoring service running")
}
