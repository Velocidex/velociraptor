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
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths/artifact_modes"
	"www.velocidex.com/golang/velociraptor/utils"
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

	// The user who is authorizing the writing of the message.  For
	// messages written by the system this will be
	// constants.VELOCIRAPTOR_SERVER_CLIENT_ID, but for user initiated
	// messages it will be the principal (or client id) who generated
	// the message. Authorization decisions are always made by this
	// Username.
	// Note that this can not be empty! It is a bug if it is.
	Username string

	// The actual user the message is from. This will always be the
	// client id or the principal who generated the message.
	// Note that this can not be empty! It is a bug if it is.
	From string

	// Event filters are applied on the event before forwarding it to
	// WatchQueueWithCB. This prevents the callback from receiving
	// invalid or untrusted events.
	EventFilter func(
		self JournalOptions,
		config_obj *config_proto.Config) error
}

func (self JournalOptions) WithSuperUser() JournalOptions {
	self.Username = constants.VELOCIRAPTOR_SERVER_CLIENT_ID
	return self
}

func (self JournalOptions) WithFrom(user string) JournalOptions {
	self.From = user
	return self
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

func (self JournalOptions) FromRow(row *ordereddict.Dict) (JournalOptions, error) {
	self.From, _ = row.GetString("_Source")
	self.Username, _ = row.GetString("_Writer")

	// Should never happen! Make sure it is obvious there is a bug.
	if self.Username == "" || self.From == "" {
		utils.PrintStack()
		return self, utils.Wrap(utils.InvalidArgError,
			"Journal parameters not set correctly for %v", self.ArtifactName)
	}
	return self, nil
}

func (self JournalOptions) TagRows(
	config_obj *config_proto.Config,
	rows []*ordereddict.Dict) error {
	// Should never happen! Make sure it is obvious there is a bug.
	if self.Username == "" || self.From == "" {
		utils.PrintStack()
		return utils.Wrap(utils.InvalidArgError,
			"Journal parameters not set correctly")
	}

	if self.EventFilter != nil {
		err := self.EventFilter(self, config_obj)
		if err != nil {
			return err
		}
	}

	for _, r := range rows {
		r.Update("_Writer", self.Username).
			Update("_Source", self.From)
	}

	return nil
}

func (self JournalOptions) TagJsonl(
	config_obj *config_proto.Config,
	jsonl []byte) ([]byte, error) {
	// Should never happen! Make sure it is obvious there is a bug.
	if self.Username == "" || self.From == "" {
		utils.PrintStack()
		return nil, utils.Wrap(utils.InvalidArgError,
			"Journal parameters not set correctly")
	}

	if self.EventFilter != nil {
		err := self.EventFilter(self, config_obj)
		if err != nil {
			return nil, err
		}
	}

	res := json.AppendJsonlItem(jsonl, "_Writer", self.Username)
	return json.AppendJsonlItem(res, "_Source", self.From), nil
}

func (self JournalOptions) Queue() artifact_modes.QueueName {
	return artifact_modes.NewQueueName(self.ArtifactName, self.ArtifactType)
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
	WatchQueue(
		ctx context.Context,
		queue_name artifact_modes.QueueName,
		watcher_name string) (output <-chan *ordereddict.Dict, cancel func())

	// A version of Watch() above for internal well known queues.
	Watch(
		ctx context.Context,
		queue JournalOptions,
		watcher_name string) (output <-chan *ordereddict.Dict, cancel func())

	GetWatchers() []artifact_modes.QueueName

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
