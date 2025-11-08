package reporting

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/uploader"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/vfilter"
)

type NotebookUploader struct {
	config_obj                 *config_proto.Config
	notebook_cell_path_manager *paths.NotebookCellPathManager
}

func (self *NotebookUploader) Upload(
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
	mode os.FileMode,
	reader io.ReadSeeker) (
	*uploads.UploadResponse, error) {

	if filename == nil {
		return nil, errors.New("Not found")
	}

	if store_as_name == nil {
		store_as_name = filename
	}

	result, closer := uploads.DeduplicateUploads(
		accessor, scope, store_as_name)
	defer closer(result)
	if result != nil {
		return result, nil
	}

	dest_path_spec := self.notebook_cell_path_manager.GetUploadsFile(
		store_as_name.String())

	// Use the filestore uploader to upload the file into the
	// filestore.
	file_store_factory := file_store.GetFileStore(self.config_obj)
	delegate_uploader := uploader.NewFileStoreUploader(
		self.config_obj, file_store_factory,
		// This is the directory that will contain the upload
		dest_path_spec.Dir())

	// Store the file in the notebook. All files will be stored as
	// flat filenames in the same directory.
	res, err := delegate_uploader.Upload(ctx, scope, filename, accessor,
		accessors.MustNewGenericOSPath("/").Append(dest_path_spec.Base()),
		expected_size, mtime, atime, ctime, btime,
		mode, reader)
	if err != nil {
		return nil, err
	}

	result = &uploads.UploadResponse{
		Path:       res.Path,
		StoredName: store_as_name.String(),
		Accessor:   accessor,
		Components: res.Components,
		Size:       res.Size,
		StoredSize: res.StoredSize,
		Sha256:     res.Sha256,
		Md5:        res.Md5,
	}
	closer(result)
	return result, nil
}
