package server_artifacts

import (
	"context"
	"io"

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
	StartRefresh()

	// A Query context track a single query in the collection.
	GetQueryContext(query *actions_proto.VQLCollectorArgs) QueryContext

	// Start the queries in other goroutines and wait for them all to
	// finish
	RunQueries(request []*actions_proto.VQLCollectorArgs) (err error)

	Save() error

	Close(ctx context.Context)

	ChargeBytes(bytes int64)
}
