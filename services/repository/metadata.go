package repository

import (
	"context"
	"sync"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
)

type metadataManager struct {
	mu sync.Mutex

	storage *artifacts_proto.ArtifactMetadataStorage
	lookup  map[string]*artifacts_proto.ArtifactMetadata
}

func (self *metadataManager) Get(name string) (
	*artifacts_proto.ArtifactMetadata, bool) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.lookup == nil {
		self.lookup = make(map[string]*artifacts_proto.ArtifactMetadata)
	}

	res, pres := self.lookup[name]
	return res, pres
}

func (self *metadataManager) Set(name string,
	metadata *artifacts_proto.ArtifactMetadata) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.lookup == nil {
		self.lookup = make(map[string]*artifacts_proto.ArtifactMetadata)
	}

	self.lookup[name] = metadata
}

func (self *metadataManager) saveMetadata(
	ctx context.Context, config_obj *config_proto.Config) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	metadata_proto := &artifacts_proto.ArtifactMetadataStorage{
		Metadata: self.lookup,
	}

	// Flush the metadata file.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	path_manager := paths.RepositoryPathManager{}
	return db.SetSubject(config_obj, path_manager.Metadata(),
		metadata_proto)
}

func (self *metadataManager) loadMetadata(
	ctx context.Context, config_obj *config_proto.Config) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	metadata := &artifacts_proto.ArtifactMetadataStorage{}
	path_manager := paths.RepositoryPathManager{}
	err = db.GetSubject(config_obj, path_manager.Metadata(), metadata)
	if err != nil {
		return err
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.lookup = metadata.Metadata

	return nil
}
