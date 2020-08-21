package services

import (
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

func GetRepositoryManager() RepositoryManager {
	repository_mu.Lock()
	defer repository_mu.Unlock()

	return grepository
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
	LoadDirectory(dirname string) (int, error)

	// Make a copy of this repository.
	Copy() Repository

	// Load definition in yaml
	LoadYaml(data string, validate bool) (*artifacts_proto.Artifact, error)

	// Load an artifact protobuf.
	LoadProto(artifact *artifacts_proto.Artifact, validate bool) (
		*artifacts_proto.Artifact, error)

	// Get an artifact by name.
	Get(name string) (*artifacts_proto.Artifact, bool)

	// Remove a named artifact from the repository.
	Del(name string)

	// List
	List() []string

	/*
		PopulateArtifactsVQLCollectorArgs(
			request *actions_proto.VQLCollectorArgs) error
	*/
}

// Manages the global artifact repository
type RepositoryManager interface {
	NewRepository() Repository
	GetGlobalRepository(config_obj *config_proto.Config) (Repository, error)
	SetGlobalRepositoryForTests(repository Repository)
	BuildScope(builder ScopeBuilder) *vfilter.Scope

	// Only used in tests - it is much more expensive. Avoids
	// caching plugin definitions.
	BuildScopeFromScratch(builder ScopeBuilder) *vfilter.Scope
	SetArtifactFile(data, required_prefix string) (*artifacts_proto.Artifact, error)
	DeleteArtifactFile(name string) error
}

func ScopeBuilderFromScope(scope *vfilter.Scope) ScopeBuilder {
	result := ScopeBuilder{
		Logger: scope.Logger,
	}
	config_obj, ok := artifacts.GetServerConfig(scope)
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
