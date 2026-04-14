package services

// Velociraptor is powered by VQL which is a query language. At its
// heart, queries simply return rows. Velociraptor organizes the
// results of queries by the artifact name - that is to say, that
// artifacts return rows when collected, which are stored in the
// datastore under the artifact name.

// The journal service organizes the rows returned from collecting an
// artifact by allowing callers to push them into the datastore (using
// a path manager to figure out exactly where).

// Similarly callers can watch for new rows to appear in any
// artifact. This allows Velociraptor server queries to receive rows
// in real time from client event artifacts.

import (
	"context"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths/artifact_modes"
)

type JournalOptions struct {
	Sync bool

	ArtifactName string
	ClientId     string
	FlowId       string

	// In many cases we already know the exact artifact we want to
	// write to. This field allows us to provide a hard coded hint to
	// save us looking the artifact up in the repository.
	ArtifactType artifact_modes.ArtifactMode

	// The user who is writing the message
	Username string

	// Event filters are applied on the event before forwarding it to
	// WatchQueueWithCB. This prevents the callback from receiving
	// invalid or untruste events.
	EventFilter func(
		config_obj *config_proto.Config,
		opts JournalOptions,
		watcher_name string,
		event *ordereddict.Dict) bool
}

func (self JournalOptions) WithUser(username string) JournalOptions {
	self.Username = username
	return self
}

func (self JournalOptions) WithClientId(client_id string) JournalOptions {
	self.ClientId = client_id
	return self
}

func (self JournalOptions) WithFlowId(flow_id string) JournalOptions {
	self.FlowId = flow_id
	return self
}

func GetJournal(config_obj *config_proto.Config) (JournalService, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	s := org_manager.Services(config_obj.OrgId)

	return s.Journal()
}

type JournalService interface {
	// Watch the artifact named by queue_name for new rows. This only
	// makes sense for artifacts of type CLIENT_EVENT and
	// SERVER_EVENT. The watcher_name is a description of who is
	// watching this particular queue.
	WatchArtifact(
		ctx context.Context,
		queue_name string,
		watcher_name string) (output <-chan *ordereddict.Dict, cancel func())

	// A version of Watch() above for internal well known queues.
	Watch(
		ctx context.Context,
		queue JournalOptions,
		watcher_name string) (output <-chan *ordereddict.Dict, cancel func())

	GetWatchers() []string

	// Push the rows into the result set in the filestore. NOTE: This
	// method synchronises access to the files within the process.
	AppendToResultSet(config_obj *config_proto.Config,
		path api.FSPathSpec, rows []*ordereddict.Dict,
		options JournalOptions) error

	Broadcast(ctx context.Context, config_obj *config_proto.Config,
		rows []*ordereddict.Dict, opts JournalOptions) error

	// Push the rows to the event artifact queue
	PushRowsToArtifact(ctx context.Context, config_obj *config_proto.Config,
		rows []*ordereddict.Dict, opts JournalOptions) error

	// An optimization around PushRowsToArtifact where rows are
	// already serialized in JSONL
	PushJsonlToArtifact(
		ctx context.Context, config_obj *config_proto.Config,
		jsonl []byte, row_count int, opts JournalOptions) error

	// Push the rows to the event artifact queue with a potential
	// unspecified delay. Internally these rows will be batched until
	// a convenient time to send them.
	PushRowsToArtifactAsync(
		ctx context.Context, config_obj *config_proto.Config,
		row *ordereddict.Dict, opts JournalOptions)
}
