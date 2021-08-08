package server_artifacts

import (
	"context"
	"io"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/vfilter"
)

type ServerUploader struct {
	*api.FileStoreUploader
	path_manager       *paths.FlowPathManager
	collection_context *contextManager
	config_obj         *config_proto.Config
}

func (self *ServerUploader) Upload(
	ctx context.Context,
	scope vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	expected_size int64,
	mtime time.Time,
	atime time.Time,
	ctime time.Time,
	btime time.Time,
	reader io.Reader) (*api.UploadResponse, error) {

	result, err := self.FileStoreUploader.Upload(ctx, scope, filename,
		accessor, store_as_name, expected_size,
		mtime, atime, ctime, btime, reader)
	if err == nil {
		journal, err := services.GetJournal()
		if err != nil {
			return nil, err
		}

		err = journal.AppendToResultSet(self.config_obj,
			self.path_manager.UploadMetadata(),
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("Timestamp", time.Now().UTC().Unix()).
				Set("started", time.Now().UTC().String()).
				Set("vfs_path", result.Path).
				Set("expected_size", result.Size).
				Set("mtime", mtime)},
		)
		if err != nil {
			return nil, err
		}

		self.collection_context.Modify(func(context *flows_proto.ArtifactCollectorContext) {
			context.TotalUploadedFiles++
			context.TotalUploadedBytes += uint64(result.Size)
			context.TotalExpectedUploadedBytes += uint64(result.Size)
		})

		return result, self.collection_context.Save()

	}
	return result, err
}

func NewServerUploader(
	config_obj *config_proto.Config,
	path_manager *paths.FlowPathManager,
	collection_context *contextManager) api.Uploader {
	return &ServerUploader{
		FileStoreUploader: api.NewFileStoreUploader(config_obj,
			file_store.GetFileStore(config_obj),
			path_specs.NewUnsafeFilestorePath()),
		path_manager:       path_manager,
		collection_context: collection_context,
		config_obj:         config_obj,
	}
}
