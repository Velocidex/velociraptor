/*

  Velociraptor relies on artifacts. Artifacts are a way of
  encapsulating VQL queries inside easy to use YAML files. These yaml
  files can be stored on disk or in the datastore.

  An artifact "Repository" is an object maintaining a self consistent
  view of a subset of known artifacts. It is self consistent in that
  artifacts may call other artifacts within the same repository.

  Artifacts are stored by name in the repository.  Repositories know
  how to parse artifacts from various sources and know how to get
  artifact definitions by name.

  The global repository is used to store all artifacts we known about
  at runtime.

  Clients do not have persistent repositories but they do create
  temporary repositories in which to run incoming queries. This allows
  VQLCollectorArgs protobufs to include dependent artifacts and have
  the client run those as well.

  The repository is an essential service and should always be running.
*/

package services

import (
	"errors"
	"log"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	repository_mu sync.Mutex
	grepository   RepositoryManager
)

func GetRepositoryManager() (RepositoryManager, error) {
	repository_mu.Lock()
	defer repository_mu.Unlock()

	if grepository == nil {
		return nil, errors.New("Repository Manager not ready")
	}

	return grepository, nil
}

func RegisterRepositoryManager(repository RepositoryManager) {
	repository_mu.Lock()
	defer repository_mu.Unlock()

	grepository = repository
}

// Make it easier to build a query scope using the aritfact
// repository.
type ScopeBuilder struct {
	Config     *config_proto.Config
	ACLManager vql_subsystem.ACLManager
	Uploader   api.Uploader
	Logger     *log.Logger
	Env        *ordereddict.Dict
	Repository Repository
}

// An artifact repository holds definitions for artifacts.
type Repository interface {
	// Load an entire directory recursively.
	LoadDirectory(config_obj *config_proto.Config, dirname string) (int, error)

	// Make a copy of this repository.
	Copy() Repository

	// Load definition in yaml
	LoadYaml(data string, validate bool) (*artifacts_proto.Artifact, error)

	// Load an artifact protobuf.
	LoadProto(artifact *artifacts_proto.Artifact, validate bool) (
		*artifacts_proto.Artifact, error)

	// Get an artifact by name.
	Get(config_obj *config_proto.Config,
		name string) (*artifacts_proto.Artifact, bool)

	// An optimization that avoids copying the entire artifact definition
	GetArtifactType(config_obj *config_proto.Config, artifact_name string) (string, error)

	// Remove a named artifact from the repository.
	Del(name string)

	// List
	List() []string
}

// Manages the global artifact repository
type RepositoryManager interface {
	// Make a new empty repository
	NewRepository() Repository

	// Get the global repository - Velociraptor uses a global
	// repository containing all artifacts it knows about. The
	// frontend loads the repository at startup from:
	//
	// 1. Build in artifacts
	// 2. Custom artifacts stored in the data store.
	// 3. Any additional artifacts directory specified in the --definitions flag.
	// Any artifacts customized via the GUI will be stored here.
	GetGlobalRepository(config_obj *config_proto.Config) (Repository, error)

	// Only used for tests.
	SetGlobalRepositoryForTests(config_obj *config_proto.Config, repository Repository)

	// Before callers can run VQL queries they need to create a
	// query scope. This function uses the builder pattern above
	// to create a new scope.
	BuildScope(builder ScopeBuilder) vfilter.Scope

	// This function is much more expensive than
	// BuildScope(). Avoids caching plugin definitions - it is
	// only useful when callers need to manipulate the scope in an
	// incompatible way - e.g. override a plugin definition.
	BuildScopeFromScratch(builder ScopeBuilder) vfilter.Scope

	// Store the file to the repository. It will be stored in the datastore as well.
	SetArtifactFile(config_obj *config_proto.Config, principal string,
		data, required_prefix string) (*artifacts_proto.Artifact, error)

	// Delete the file from the global repository and the data store.
	DeleteArtifactFile(config_obj *config_proto.Config,
		principal, name string) error
}

// A helper function to build a new scope from an existing scope. This
// is needed in order to isolate the existing scope from the new scope
// (e.g. when running a sub-artifact)
func ScopeBuilderFromScope(scope vfilter.Scope) ScopeBuilder {
	result := ScopeBuilder{
		Logger: scope.GetLogger(),
	}
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if ok {
		result.Config = config_obj
	}

	uploader, ok := artifacts.GetUploader(scope)
	if ok {
		result.Uploader = uploader
	}

	acl_manger, ok := artifacts.GetACLManager(scope)
	if ok {
		result.ACLManager = acl_manger
	}

	return result
}
