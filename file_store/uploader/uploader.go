package uploader

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/vfilter"
)

// An uploader into the filestore.
type FileStoreUploader struct {
	file_store api.FileStore
	root_path  api.FSPathSpec
}

func (self *FileStoreUploader) Upload(
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
	reader io.Reader) (
	*uploads.UploadResponse, error) {

	if store_as_name == nil {
		store_as_name = filename
	}

	output_path := self.root_path.AddUnsafeChild(store_as_name.Components...)
	out_fd, err := self.file_store.WriteFile(output_path)
	if err != nil {
		scope.Log("Unable to open file %s: %v",
			store_as_name.String(), err)
		return nil, err
	}
	defer out_fd.Close()

	err = out_fd.Truncate()
	if err != nil {
		scope.Log("Unable to truncate file %s: %v",
			store_as_name.String(), err)
		return nil, err
	}

	buf := make([]byte, 1024*1024)
	offset := int64(0)
	md5_sum := md5.New()
	sha_sum := sha256.New()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop

		default:
			n, err := reader.Read(buf)
			if n == 0 || err == io.EOF {
				break loop
			}
			data := buf[:n]

			_, err = out_fd.Write(data)
			if err != nil {
				return nil, err
			}

			_, _ = md5_sum.Write(data)
			_, _ = sha_sum.Write(data)

			offset += int64(n)
		}
	}

	// Return paths relative to the storage root.
	relative_path := path_specs.NewUnsafeFilestorePath(store_as_name.Components...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)

	scope.Log("Uploaded %v (%v bytes)", relative_path.AsClientPath(), offset)
	return &uploads.UploadResponse{
		Path:       relative_path.AsClientPath(),
		Size:       uint64(offset),
		StoredSize: uint64(offset),
		Sha256:     hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:        hex.EncodeToString(md5_sum.Sum(nil)),
		// Full components to the file in Components
		Components: output_path.Components(),
	}, nil
}

func NewFileStoreUploader(
	config_obj *config_proto.Config,
	fs api.FileStore,
	root_path api.FSPathSpec) *FileStoreUploader {
	return &FileStoreUploader{fs, root_path}
}
