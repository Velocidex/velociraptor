package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type metadataManager struct {
	mu sync.Mutex

	ctx        context.Context
	config_obj *config_proto.Config

	lookup map[string]*artifacts_proto.ArtifactMetadata

	dirty      bool
	last_write time.Time

	repository services.Repository
	id         uint64
}

func NewMetadataManager(
	ctx context.Context,
	config_obj *config_proto.Config) *metadataManager {

	res := &metadataManager{
		ctx:        ctx,
		config_obj: config_obj,
		id:         utils.GetId(),
	}

	// It is not an error if there is no metadata file yet.
	_ = res.loadMetadata(ctx, config_obj)
	if res.lookup == nil {
		res.lookup = make(map[string]*artifacts_proto.ArtifactMetadata)
	}

	for _, wk := range artifacts.WELL_KNOWN_QUEUES {
		res.lookup[wk.ArtifactName] = &artifacts_proto.ArtifactMetadata{
			Hidden: true,
		}
	}

	return res
}

func (self *metadataManager) Tags() (res []string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	lookup := make(map[string]bool)
	for _, v := range self.lookup {
		for _, t := range v.Tags {
			lookup[t] = true
		}
	}

	for k := range lookup {
		res = append(res, k)
	}

	sort.Strings(res)

	return res
}

func (self *metadataManager) Clear(name string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.lookup, name)
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

func (self *metadataManager) Set(
	name string, metadata *artifacts_proto.ArtifactMetadata) {

	self.mu.Lock()
	if self.lookup == nil {
		self.lookup = make(map[string]*artifacts_proto.ArtifactMetadata)
	}

	self.lookup[name] = metadata
	self.dirty = true

	last_write := self.last_write
	repository := self.repository
	self.mu.Unlock()

	if utils.GetTime().Now().Sub(last_write) > time.Second &&
		repository != nil {
		_ = self.SaveMetadata(self.ctx, self.config_obj, self.repository)
	}
}

func (self *metadataManager) HouseKeeping(
	ctx context.Context, config_obj *config_proto.Config,
	wg *sync.WaitGroup, repository services.Repository) {
	defer wg.Done()

	for {
		last_try := utils.GetTime().Now()

		self.mu.Lock()
		self.repository = repository
		self.mu.Unlock()

		select {
		case <-ctx.Done():
			return

		case <-utils.GetTime().After(utils.Jitter(time.Minute)):
			// Avoid retrying too quickly. This is mainly for
			// tests where the time is mocked for the After(delay)
			// above does not work.
			if utils.GetTime().Now().Sub(last_try) < time.Second*10 {
				utils.SleepWithCtx(ctx, time.Minute)
				continue
			}

			_ = self.SaveMetadata(ctx, config_obj, repository)
		}
	}
}

func (self *metadataManager) SaveMetadata(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		// Not an error - without a datastore run with an in-memory
		// repository.
		self.dirty = false
		self.last_write = utils.GetTime().Now()
		return nil
	}

	path_manager := paths.RepositoryPathManager{}

	// Merge the existing records from the datastore.
	existing := &artifacts_proto.ArtifactMetadataStorage{}
	_ = db.GetSubject(config_obj, path_manager.Metadata(), existing)

	if existing.Metadata == nil {
		existing.Metadata = make(map[string]*artifacts_proto.ArtifactMetadata)
	}

	for k, v := range self.lookup {
		existing.Metadata[k] = v
	}

	// Filter the metadata to only contain existing artifacts - this
	// is needed to remove metadata from deleted artifacts.
	new_lookup := make(map[string]*artifacts_proto.ArtifactMetadata)

	artifacts, err := repository.List(ctx, config_obj)
	if err != nil {
		return err
	}

	for _, artifact_name := range artifacts {
		md, pres := existing.Metadata[artifact_name]
		if pres {
			new_lookup[artifact_name] = md
		}

		md, pres = self.lookup[artifact_name]
		if pres {
			new_lookup[artifact_name] = md
		}
	}
	self.lookup = new_lookup

	metadata_proto := &artifacts_proto.ArtifactMetadataStorage{
		Metadata: self.lookup,
	}

	// Flush the metadata file.
	err = db.SetSubject(config_obj, path_manager.Metadata(),
		metadata_proto)

	self.dirty = false
	self.last_write = utils.GetTime().Now()

	return err
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
