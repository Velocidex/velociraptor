package services

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	NOTIFY_CLIENT = true
	ReallyDoIt    = true

	Unknown ClientOS = iota
	Windows
	Linux
	MacOS

	SERVER_OK = true
)

var (
	DiscardDeleteProgress chan DeleteFlowResponse = nil
)

type ClientOS int

func NewClientOS(os string) ClientOS {
	switch strings.ToLower(os) {
	case "windows":
		return Windows
	case "linux":
		return Linux
	case "darwin":
		return MacOS
	}
	return Unknown
}

// Keep some stats about the client in the cache. These will be synced
// to disk periodically.
type Stats struct {
	Ping                  uint64 `json:"Ping,omitempty"`
	LastHuntTimestamp     uint64 `json:"LastHuntTimestamp,omitempty"`
	LastEventTableVersion uint64 `json:"LastEventTableVersion,omitempty"`
	IpAddress             string `json:"IpAddress,omitempty"`
}

func GetClientInfoManager(config_obj *config_proto.Config) (ClientInfoManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).ClientInfoManager()
}

type ClientInfo struct {
	// The original info from disk
	*actions_proto.ClientInfo
}

func (self *ClientInfo) Copy() *ClientInfo {
	copy := proto.Clone(self.ClientInfo).(*actions_proto.ClientInfo)
	return &ClientInfo{ClientInfo: copy}
}

func (self *ClientInfo) OS() ClientOS {
	return NewClientOS(self.System)
}

type ClientInfoManager interface {
	ListClients(ctx context.Context) <-chan string

	// Used to set a new client record. To modify an existing record -
	// or set a new one use Modify()
	Set(ctx context.Context,
		client_info *ClientInfo) error

	// Modify a record or set a new one - if the record is not found,
	// modifier will receive a nil client_info. The ClientInfoManager
	// can not be accessed within the modifier function as it is
	// locked for the duration of the change.  The Callback can
	// indicate no change is needed by returning a nil for
	// client_info.
	Modify(ctx context.Context, client_id string,
		modifier func(client_info *ClientInfo) (new_record *ClientInfo, err error)) error

	// Gets a **copy** of the client record. This is expensive -
	// consider using Modify to get only the fields you want.
	Get(ctx context.Context,
		client_id string) (*ClientInfo, error)

	Remove(ctx context.Context, client_id string)

	GetStats(ctx context.Context, client_id string) (*Stats, error)
	UpdateStats(ctx context.Context, client_id string, stats *Stats) error

	// Get the client's tasks and remove them from the queue.
	GetClientTasks(ctx context.Context,
		client_id string) ([]*crypto_proto.VeloMessage, error)

	// Get all the tasks without de-queuing them.
	PeekClientTasks(ctx context.Context,
		client_id string) ([]*crypto_proto.VeloMessage, error)

	QueueMessagesForClient(
		ctx context.Context,
		client_id string,
		req []*crypto_proto.VeloMessage,
		notify bool, /* Also notify the client about the new task */
	) error

	QueueMessageForClient(
		ctx context.Context,
		client_id string,
		req *crypto_proto.VeloMessage,
		notify bool, /* Also notify the client about the new task */
		completion func()) error

	UnQueueMessageForClient(
		ctx context.Context,
		client_id string,
		req *crypto_proto.VeloMessage) error

	// Gets the client metadata. Generally client should avoid the
	// get/modify/set pattern and use the ModifyMetadata() function
	// below instead.
	GetMetadata(ctx context.Context,
		client_id string) (*ordereddict.Dict, error)

	// Sets the new metadata into the client metadata. Keys in the
	// new_metadata will be merged into the existing metadata (dict
	// addition). If the values are NULL then the keys are removed
	// from the metadata. This merge operation is done atomically.
	SetMetadata(ctx context.Context,
		client_id string, new_metadata *ordereddict.Dict,
		principal string) error

	// Modify the metadata atomically. This avoids the get/modify/set
	// pattern and is race free.  The Callback can indicate no change
	// is needed by returning a nil for metadata.
	ModifyMetadata(ctx context.Context, config_obj *config_proto.Config,
		client_id, principal string, cb func(*ordereddict.Dict) (
			new_metadata *ordereddict.Dict, err error)) error

	// Make sure the client_id is validly formed. If server_ok is
	// specified, we also allow the special client id of "server"
	// representing the server.
	ValidateClientId(client_id string, server_ok bool) error

	DeleteClient(ctx context.Context, client_id, principal string,
		progress chan DeleteFlowResponse, really_do_it bool) error
}

func GetHostname(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) string {

	hostname, err := GetClientInfoItemString(ctx, config_obj, client_id,
		func(record *actions_proto.ClientInfo) (string, error) {
			return record.Hostname, nil
		})
	if err == nil {
		return hostname
	}

	return ""
}

// A helper function to safely copy parts of the client record under
// lock.
func GetClientInfoItem(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	cb func(record *actions_proto.ClientInfo) (interface{}, error)) (result interface{}, err error) {
	client_info_manager, err := GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	err1 := client_info_manager.Modify(ctx, client_id,
		func(client_info *ClientInfo) (new_record *ClientInfo, err error) {
			if client_info == nil {
				return nil, utils.NotFoundError
			}

			if cb != nil {
				result, err = cb(client_info.ClientInfo)
			}
			return nil, nil
		})

	if err1 != nil {
		return nil, err1
	}
	return result, nil
}

func GetClientInfoItemString(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	cb func(record *actions_proto.ClientInfo) (string, error)) (result string, err error) {
	res, err := GetClientInfoItem(ctx, config_obj, client_id,
		func(record *actions_proto.ClientInfo) (interface{}, error) {
			return cb(record)
		})
	if err != nil {
		return "", err
	}
	res_str, ok := res.(string)
	if !ok {
		return "", utils.InvalidArgError
	}

	return res_str, nil
}

var (
	CheckForPresence func(record *actions_proto.ClientInfo) (interface{}, error) = nil
)
