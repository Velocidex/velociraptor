package ddclient

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type Updater interface {
	UpdateDDNSRecord(
		ctx context.Context, config_obj *config_proto.Config,
		external_ip string) error
}
