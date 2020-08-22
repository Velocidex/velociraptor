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
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	journal_mu sync.Mutex

	// Service is only available in the frontend.
	GJournal JournalService
)

func GetJournal() JournalService {
	journal_mu.Lock()
	defer journal_mu.Unlock()

	return GJournal
}

func RegisterJournal(journal JournalService) {
	journal_mu.Lock()
	defer journal_mu.Unlock()

	GJournal = journal
}

type JournalService interface {
	// Watch the artifact named by queue_name for new rows. This
	// only makes sense for artifacts of type CLIENT_EVENT and
	// SERVER_EVENT
	Watch(queue_name string) (output <-chan *ordereddict.Dict, cancel func())

	// Push the rows into the datastore in the location give by
	// the path manager.
	PushRows(path_manager api.PathManager, rows []*ordereddict.Dict) error

	// Push the rows to the event artifact queue
	PushRowsToArtifact(rows []*ordereddict.Dict, name, client_id, flows_id string) error
}
