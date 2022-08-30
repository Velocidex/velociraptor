package server_artifacts

import (
	"context"
	"io"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/uploader"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/vfilter"
)

type ServerUploader struct {
	*uploader.FileStoreUploader
	path_manager       *paths.FlowPathManager
	collection_context CollectionContextManager
	config_obj         *config_proto.Config
}

func (self *ServerUploader) Upload(
	ctx context.Context,
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor string,
	store_as_name string,
	expected_size int64,
	mtime time.Time,
	atime time.Time,
	ctime time.Time,
	btime time.Time,
	reader io.Reader) (*uploads.UploadResponse, error) {

	result, err := self.FileStoreUploader.Upload(ctx, scope, filename,
		accessor, store_as_name, expected_size,
		mtime, atime, ctime, btime, reader)
	if err == nil {
		journal, err := services.GetJournal(self.config_obj)
		if err != nil {
			return nil, err
		}

		timestamp := time.Now().UTC().Unix()
		err = journal.AppendToResultSet(self.config_obj,
			self.path_manager.UploadMetadata(),
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("Timestamp", timestamp).
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

		row := ordereddict.NewDict().
			Set("Timestamp", timestamp).
			Set("ClientId", "server").
			Set("VFSPath", result.Path).
			Set("UploadName", store_as_name).
			Set("Accessor", "fs").
			Set("Size", result.Size).
			Set("UploadedSize", result.Size)

		err = journal.PushRowsToArtifact(self.config_obj,
			[]*ordereddict.Dict{row},
			"System.Upload.Completion",
			"server", self.collection_context.GetContext().SessionId,
		)
		if err != nil {
			return nil, err
		}

		return result, self.collection_context.Save()

	}
	return result, err
}

func NewServerUploader(
	config_obj *config_proto.Config,
	path_manager *paths.FlowPathManager,
	collection_context CollectionContextManager) uploads.Uploader {
	return &ServerUploader{
		FileStoreUploader: uploader.NewFileStoreUploader(config_obj,
			file_store.GetFileStore(config_obj),
			path_manager.UploadContainer()),
		path_manager:       path_manager,
		collection_context: collection_context,
		config_obj:         config_obj,
	}
}
