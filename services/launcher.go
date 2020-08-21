package services

// Many parts of Velociraptor require launching new collections. The
// logic for preparing, verifying, Compiling and launching new
// collections is extracted into a service so it can be accessible
// from many components.

// Most callers will only need to call ScheduleArtifactCollection()
// which does all the required steps and launches the collection.

import (
	"context"
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	launcher_mu sync.Mutex
	g_launcher  Launcher = nil
)

func GetLauncher() Launcher {
	launcher_mu.Lock()
	defer launcher_mu.Unlock()

	return g_launcher
}

func RegisterLauncher(l Launcher) {
	launcher_mu.Lock()
	defer launcher_mu.Unlock()

	g_launcher = l
}

type Launcher interface {
	SetFlowIdForTests(flow_id string)

	// Check any declared tools exist and are available - possibly
	// by downloading them.
	EnsureToolsDeclared(
		ctx context.Context,
		artifact *artifacts_proto.Artifact) error

	// Compiles an ArtifactCollectorArgs (for example as passed
	// into CreateHunt() or CollectArtifact() API into a list of
	// VQLCollectorArgs - the messages sent to the client to
	// actually collect the artifact. On the client a
	// VQLCollectorArgs is collected serially in a single
	// goroutine. This means all the artifacts in the
	// ArtifactCollectorArgs will be collected one after the other
	// in turn. If called want to collect artifacts in parallel
	// then they need to perpare several VQLCollectorArgs and
	// launch them as separate messages.

	// This method is only useful when the caller wants to cache
	// the compilation process once and run it many times (e.g. in
	// a hunt).
	CompileCollectorArgs(
		ctx context.Context,
		acl_manager vql_subsystem.ACLManager,
		repository Repository,
		collector_request *flows_proto.ArtifactCollectorArgs) (
		*actions_proto.VQLCollectorArgs, error)

	// Main entry point to launch an artifact collection.
	ScheduleArtifactCollection(
		ctx context.Context,
		acl_manager vql_subsystem.ACLManager,
		repository Repository,
		collector_request *flows_proto.ArtifactCollectorArgs) (string, error)
}
