package server_artifacts

import (
	"context"
	"io"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/uploader"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type ServerUploader struct {
	*uploader.FileStoreUploader
	path_manager  *paths.FlowPathManager
	query_context QueryContext
	config_obj    *config_proto.Config
	session_id    string
}

func (self *ServerUploader) Upload(
	ctx context.Context,
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor string,
	store_as_name *accessors.OSPath,
	expected_size int64,
	mtime time.Time,
	atime time.Time,
	ctime time.Time,
	btime time.Time,
	reader io.Reader) (*uploads.UploadResponse, error) {

	result, err := self.FileStoreUploader.Upload(ctx, scope, filename,
		accessor, store_as_name, expected_size,
		mtime, atime, ctime, btime, reader)
	if err != nil {
		return nil, err
	}

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return nil, err
	}

	timestamp := utils.GetTime().Now().UTC().Unix()
	err = journal.AppendToResultSet(self.config_obj,
		self.path_manager.UploadMetadata(),
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("Timestamp", timestamp).
			Set("started", utils.GetTime().Now().UTC().String()).
			Set("vfs_path", result.Path).
			Set("_Components", result.Components).
			Set("file_size", result.Size).
			Set("uploaded_size", result.Size),
		},
	)
	if err != nil {
		return nil, err
	}

	self.query_context.UpdateStatus(func(s *crypto_proto.VeloStatus) {
		s.UploadedFiles++
		s.UploadedBytes += int64(result.Size)
		s.ExpectedUploadedBytes += int64(result.Size)
	})

	row := ordereddict.NewDict().
		Set("Timestamp", timestamp).
		Set("ClientId", "server").
		Set("VFSPath", result.Path).
		Set("UploadName", store_as_name.String()).
		Set("Accessor", "fs").
		Set("Size", result.Size).
		Set("UploadedSize", result.Size)

	err = journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{row},
		"System.Upload.Completion",
		"server", self.session_id,
	)
	return result, err
}

func NewServerUploader(
	config_obj *config_proto.Config,
	session_id string,
	path_manager *paths.FlowPathManager,
	query_context QueryContext) uploads.Uploader {
	return &ServerUploader{
		FileStoreUploader: uploader.NewFileStoreUploader(config_obj,
			file_store.GetFileStore(config_obj),
			path_manager.UploadContainer()),
		path_manager:  path_manager,
		query_context: query_context,
		config_obj:    config_obj,
		session_id:    session_id,
	}
}
