package services

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type ServerUploader struct {
	*api.FileStoreUploader
	path_manager       *paths.FlowPathManager
	collection_context *flows_proto.ArtifactCollectorContext
	config_obj         *config_proto.Config
}

func (self *ServerUploader) Upload(
	ctx context.Context,
	scope *vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	expected_size int64,
	reader io.Reader) (*api.UploadResponse, error) {

	// The server may write to the root of the filestore by
	// prefixing the store_as_name with fs://
	if strings.HasPrefix(store_as_name, "fs://") {
		store_as_name = strings.TrimPrefix(store_as_name, "fs://")
	} else {
		store_as_name = self.path_manager.GetUploadsFile(accessor, store_as_name).Path()
	}

	result, err := self.FileStoreUploader.Upload(ctx, scope, filename,
		accessor, store_as_name, expected_size, reader)
	utils.Debug(result)
	utils.Debug(err)
	utils.Debug(self.path_manager.UploadMetadata().Path())
	if err == nil {
		GetJournal().PushRows(self.path_manager.UploadMetadata(),
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("Timestamp", time.Now().UTC().Unix()).
				Set("started", time.Now().UTC().String()).
				Set("vfs_path", result.Path).
				Set("expected_size", result.Size)},
		)
		self.collection_context.TotalUploadedFiles++
		self.collection_context.TotalUploadedBytes += result.Size
		self.collection_context.TotalExpectedUploadedBytes += result.Size
		err = self.flushContext()

	}
	return result, err
}

func (self *ServerUploader) flushContext() error {
	// Write the data before we fire the event.
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		self.collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		self.collection_context.Status = err.Error()
	}

	return db.SetSubject(self.config_obj,
		self.path_manager.Path(), self.collection_context)
}

func NewServerUploader(
	config_obj *config_proto.Config,
	path_manager *paths.FlowPathManager,
	collection_context *flows_proto.ArtifactCollectorContext) api.Uploader {
	return &ServerUploader{
		FileStoreUploader: api.NewFileStoreUploader(config_obj,
			file_store.GetFileStore(config_obj), "/"),
		path_manager:       path_manager,
		collection_context: collection_context,
		config_obj:         config_obj,
	}
}
