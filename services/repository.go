package services

import (
	"sync"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
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

// Manages the global artifact repository
type RepositoryManager interface {
	SetArtifactFile(data, required_prefix string) (*artifacts_proto.Artifact, error)
	DeleteArtifactFile(name string) error
}
