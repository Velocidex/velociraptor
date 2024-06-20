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
)

type JournalOptions struct {
	Sync bool
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
	Watch(
		ctx context.Context,
		queue_name string,
		watcher_name string) (output <-chan *ordereddict.Dict, cancel func())

	GetWatchers() []string

	// Push the rows into the result set in the filestore. NOTE: This
	// method synchronises access to the files within the process.
	AppendToResultSet(config_obj *config_proto.Config,
		path api.FSPathSpec, rows []*ordereddict.Dict,
		options JournalOptions) error

	Broadcast(ctx context.Context, config_obj *config_proto.Config,
		rows []*ordereddict.Dict, name, client_id, flows_id string) error

	// Push the rows to the event artifact queue
	PushRowsToArtifact(ctx context.Context, config_obj *config_proto.Config,
		rows []*ordereddict.Dict, name, client_id, flows_id string) error

	// An optimization around PushRowsToArtifact where rows are
	// already serialized in JSONL
	PushJsonlToArtifact(
		ctx context.Context, config_obj *config_proto.Config,
		jsonl []byte, row_count int,
		name, client_id, flows_id string) error

	// Push the rows to the event artifact queue with a potential
	// unspecified delay. Internally these rows will be batched until
	// a convenient time to send them.
	PushRowsToArtifactAsync(
		ctx context.Context, config_obj *config_proto.Config,
		row *ordereddict.Dict, name string)
}
