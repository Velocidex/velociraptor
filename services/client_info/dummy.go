package client_info

import (
	"context"

	"github.com/Velocidex/ordereddict"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DummyClientInfoManager struct{}

func (self DummyClientInfoManager) ListClients(ctx context.Context) <-chan string {
	output_chan := make(chan string)
	close(output_chan)
	return output_chan
}

// Used to set a new client record. To modify an existing record -
// or set a new one use Modify()
func (self DummyClientInfoManager) Set(ctx context.Context,
	client_info *services.ClientInfo) error {
	return utils.NotImplementedError
}

// Modify a record or set a new one - if the record is not found,
// modifier will receive a nil client_info. The ClientInfoManager
// can not be accessed within the modifier function as it is
// locked for the duration of the change.
func (self DummyClientInfoManager) Modify(ctx context.Context, client_id string,
	modifier func(client_info *services.ClientInfo) (
		new_record *services.ClientInfo, err error)) error {
	return utils.NotImplementedError
}

func (self DummyClientInfoManager) Get(ctx context.Context,
	client_id string) (*services.ClientInfo, error) {
	return nil, utils.NotImplementedError
}

func (self DummyClientInfoManager) Remove(ctx context.Context, client_id string) {}

func (self DummyClientInfoManager) GetStats(
	ctx context.Context, client_id string) (*services.Stats, error) {
	return nil, utils.NotImplementedError
}

func (self DummyClientInfoManager) UpdateStats(
	ctx context.Context, client_id string, stats *services.Stats) error {
	return utils.NotImplementedError
}

// Get the client's tasks and remove them from the queue.
func (self DummyClientInfoManager) GetClientTasks(ctx context.Context,
	client_id string) ([]*crypto_proto.VeloMessage, error) {
	return nil, utils.NotImplementedError
}

// Get all the tasks without de-queuing them.
func (self DummyClientInfoManager) PeekClientTasks(ctx context.Context,
	client_id string) ([]*crypto_proto.VeloMessage, error) {
	return nil, utils.NotImplementedError
}

func (self DummyClientInfoManager) QueueMessagesForClient(
	ctx context.Context,
	client_id string,
	req []*crypto_proto.VeloMessage,
	notify bool, /* Also notify the client about the new task */
) error {
	return utils.NotImplementedError
}

func (self DummyClientInfoManager) QueueMessageForClient(
	ctx context.Context,
	client_id string,
	req *crypto_proto.VeloMessage,
	notify bool, /* Also notify the client about the new task */
	completion func()) error {
	return utils.NotImplementedError
}

func (self DummyClientInfoManager) UnQueueMessageForClient(
	ctx context.Context,
	client_id string,
	req *crypto_proto.VeloMessage) error {
	return utils.NotImplementedError
}

// Be able to manipulate the client and server metadata.
func (self DummyClientInfoManager) GetMetadata(ctx context.Context,
	client_id string) (*ordereddict.Dict, error) {
	return nil, utils.NotImplementedError
}

func (self DummyClientInfoManager) SetMetadata(ctx context.Context,
	client_id string, metadata *ordereddict.Dict, principal string) error {
	return utils.NotImplementedError
}

func (self DummyClientInfoManager) ValidateClientId(client_id string) error {
	return utils.NotImplementedError
}

func (self DummyClientInfoManager) DeleteClient(
	ctx context.Context, client_id, principal string,
	progress chan services.DeleteFlowResponse, really_do_it bool) error {
	return utils.NotImplementedError
}
