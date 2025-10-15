package services

// Many parts of Velociraptor require launching new collections. The
// logic for preparing, verifying, Compiling and launching new
// collections is extracted into a service so it can be accessible
// from many components.

// Velociraptor treats all input as artifacts - users can launch new
// artifact collection on endpoints by naming the artifact and
// providing parameters. However the endpoint itself is not directly
// running the artifacts - it simply runs VQL statements. We do this
// so that artifacts can be edited and customized on the server
// without needing to deploy new clients.

// On the server, collections are created using ArtifactCollectorArgs
// On the client, VQL is executing from VQLCollectorArgs

// Ultimately the launcher is responsible for compiling the requested
// ArtifactCollectorArgs collection into the VQLCollectorArgs protobuf
// that will be sent to the client. Compiling the artifact means:

// 1. Converting the artifact definition into a sequence of VQL

// 2. Populating the query environment from the artifact definition
//    defaults and merging the user's parameters into the initial
//    query environment.

// 3. Include any dependent artifacts in the VQLCollectorArgs. On the
//    client, these additional artifacts will be compiled into a
//    temporary artifact repository for execution (i.e. the client
//    never uses its built in artifacts).

// 4. Adding any required tools by the artifact and filling in their
//    tool details (required hash, and download location).

// Most callers will only need to call ScheduleArtifactCollection()
// which does all the required steps and launches the collection.

// It is possible for callers to pre-compile the artifact and cache
// the VQLCollectorArgs for later use to avoid the cost of compiling
// the artifact. This is useful e.g. in hunts to be able to scale the
// launching of similar collections on many hosts at the same time.

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

const (
	// When the principal is set to this below we avoid audit logging
	// the call.
	NoAuditLogging = ""
)

var (
	FlowNotFoundError = utils.Wrap(utils.NotFoundError, "Flow not found")
	DryRunOnly        = DeleteFlowOptions{
		ReallyDoIt: false,
	}
)

type DeleteFlowResponse struct {
	Id    int               `json:"-"`
	Type  string            `json:"type"`
	Data  *ordereddict.Dict `json:"data"`
	Error string            `json:"error"`
}

func GetLauncher(config_obj *config_proto.Config) (Launcher, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).Launcher()
}

// Options for the GetFlowOptions API. This ensures we do no more work
// than necessary.
type GetFlowOptions struct {

	// Include the flow downloads (ZIP exports of the flow).
	Downloads bool
}

type DeleteFlowOptions struct {
	// If this is not set, we do a dry run to indicate which files
	// will be deleted within the flow but do not actually delete the
	// files.
	ReallyDoIt bool

	// If this is set the delete will be synchronous and index updated
	// immediately. This is much slower but it is necessary when
	// results need to be available immediately. When False, we delete
	// asynchronously and update the index at a later time.
	Sync bool
}

type CompilerOptions struct {
	// Should names be obfuscated in the resulting VQL?
	ObfuscateNames bool

	// Generate precondition queries.
	DisablePrecondition bool

	// Ignore Missing Artifacts without raising an error.
	IgnoreMissingArtifacts bool

	LogBatchTime uint64
}

// Written in the flow index
type FlowSummary struct {
	FlowId    string   `json:"FlowId"`
	Artifacts []string `json:"Artifacts"`
	Created   uint64   `json:"Created"`
	Creator   string   `json:"Creator"`
}

type FlowStorer interface {
	WriteFlow(
		ctx context.Context,
		config_obj *config_proto.Config,
		flow *flows_proto.ArtifactCollectorContext,
		completion func()) error

	WriteFlowStats(
		ctx context.Context,
		config_obj *config_proto.Config,
		flow *flows_proto.ArtifactCollectorContext,
		completion func()) error

	WriteFlowIndex(
		ctx context.Context,
		config_obj *config_proto.Config,
		flow *flows_proto.ArtifactCollectorContext) error

	WriteTask(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string,
		msg *crypto_proto.VeloMessage) error

	DeleteFlow(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string, flow_id string, principal string,
		options DeleteFlowOptions) ([]*DeleteFlowResponse, error)

	LoadCollectionContext(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error)

	ListFlows(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string,
		options result_sets.ResultSetOptions,
		offset, length int64) ([]*FlowSummary, int64, error)

	// Get the exact requests that were sent for this collection (for
	// provenance).
	GetFlowRequests(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string, flow_id string,
		offset uint64, count uint64) (*api_proto.ApiFlowRequestDetails, error)
}

type Launcher interface {
	Storage() FlowStorer

	// Check any declared tools exist and are available - possibly
	// by downloading them.
	EnsureToolsDeclared(
		ctx context.Context,
		config_obj *config_proto.Config,
		artifact *artifacts_proto.Artifact) error

	// Calculates the dependent artifacts
	GetDependentArtifacts(
		ctx context.Context, config_obj *config_proto.Config,
		repository Repository,
		names []string) ([]string, error)

	// Compiles an ArtifactCollectorArgs (for example as passed
	// into CreateHunt() or CollectArtifact() API into a list of
	// VQLCollectorArgs - the messages sent to the client to
	// actually collect the artifact. On the client a
	// VQLCollectorArgs is collected serially in a single
	// goroutine. This means all the artifacts in the
	// ArtifactCollectorArgs will be collected one after the other
	// in turn. If callers want to collect artifacts in parallel
	// then they need to perpare several VQLCollectorArgs and
	// launch them as separate messages.

	// This method is only useful when the caller wants to cache
	// the compilation process once and run it many times (e.g. in
	// a hunt).
	CompileCollectorArgs(
		ctx context.Context,
		config_obj *config_proto.Config,
		acl_manager vql_subsystem.ACLManager,
		repository Repository,
		options CompilerOptions,
		collector_request *flows_proto.ArtifactCollectorArgs) (
		[]*actions_proto.VQLCollectorArgs, error)

	// Take the compiled requests from above and schedule them on the
	// client.
	WriteArtifactCollectionRecord(
		ctx context.Context,
		config_obj *config_proto.Config,
		collector_request *flows_proto.ArtifactCollectorArgs,
		vql_collector_args []*actions_proto.VQLCollectorArgs,
		completion func(task *crypto_proto.VeloMessage)) (string, error)

	// Main entry point to launch an artifact collection.
	ScheduleArtifactCollection(
		ctx context.Context,
		config_obj *config_proto.Config,
		acl_manager vql_subsystem.ACLManager,
		repository Repository,
		collector_request *flows_proto.ArtifactCollectorArgs,
		completion func()) (string, error)

	// The following methods are used to manage collections

	// Get a list of collections summary from a client.
	GetFlows(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string,
		options result_sets.ResultSetOptions,
		offset, length int64) (*api_proto.ApiFlowResponse, error)

	// Get the details of a flow - this has a lot more information
	// than the previous method.
	GetFlowDetails(
		ctx context.Context,
		config_obj *config_proto.Config,
		opts GetFlowOptions,
		client_id string, flow_id string) (*api_proto.FlowDetails, error)

	// Actively cancel the collection
	CancelFlow(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id, flow_id, principal string) (
		res *api_proto.StartFlowResponse, err error)

	// Replay flow transactions to continue if possible.
	ResumeFlow(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id, flow_id string) (
		[]*actions_proto.UploadTransaction, error)

	DeleteEvents(
		ctx context.Context,
		config_obj *config_proto.Config,
		principal, artifact, client_id string,
		start_time, end_time time.Time,
		options DeleteFlowOptions) ([]*DeleteFlowResponse, error)
}
