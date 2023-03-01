package server_artifacts

import (
	"context"
	"io"
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

type LogWriter interface {
	io.Writer
	Close()
}
type QueryContext interface {
	// Get a logger we can pass to the scope. Logs are managed per
	// query.
	Logger() LogWriter

	UpdateStatus(cb func(s *crypto_proto.VeloStatus))
	GetStatus() *crypto_proto.VeloStatus

	// Close the query and update its collection context.
	Close()
}

type CollectionContextManager interface {
	Logger() LogWriter

	GetContext() *flows_proto.ArtifactCollectorContext

	// Loads the context from storage.
	Load() error

	// Start writing the collection context to storage.
	StartRefresh(wg *sync.WaitGroup)

	// A Query context track a single query in the collection.
	GetQueryContext(query *actions_proto.VQLCollectorArgs) QueryContext

	RunQuery(arg *actions_proto.VQLCollectorArgs) error

	Save() error

	// Cancel all the queries in this collection immediately and wait
	// for them to complete
	Cancel(ctx context.Context, princiapl string)

	Close(ctx context.Context)

	ChargeBytes(bytes int64)
}
