package api

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/vfilter"
)

// Returned as the result of the query.
type UploadResponse struct {
	Path       string `json:"Path"`
	Size       uint64 `json:"Size"`
	StoredSize uint64 `json:"StoredSize,omitempty"`
	Error      string `json:"Error,omitempty"`
	Sha256     string `json:"sha256,omitempty"`
	Md5        string `json:"md5,omitempty"`
	StoredName string `json:"StoredName,omitempty"`

	// Added when we store to a container to indicate where in the zip
	// file the upload ended up.
	ContainerPath string `json:"ContainerPath,omitempty"`
}

// Provide an uploader capable of uploading any reader object.
type Uploader interface {
	Upload(ctx context.Context,
		scope vfilter.Scope,
		filename string,
		accessor string,
		store_as_name string,
		expected_size int64,
		mtime time.Time,
		atime time.Time,
		ctime time.Time,
		btime time.Time,
		reader io.Reader) (*UploadResponse, error)
}

// An uploader into the filestore.
type FileStoreUploader struct {
	file_store FileStore
	root_path  FSPathSpec
}

func (self *FileStoreUploader) Upload(
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
	reader io.Reader) (
	*UploadResponse, error) {

	if store_as_name == "" {
		store_as_name = filename
	}

	output_path := self.root_path.AddUnsafeChild(store_as_name)
	out_fd, err := self.file_store.WriteFile(output_path)
	if err != nil {
		scope.Log("Unable to open file %s: %v", store_as_name, err)
		return nil, err
	}
	defer out_fd.Close()

	err = out_fd.Truncate()
	if err != nil {
		scope.Log("Unable to truncate file %s: %v", store_as_name, err)
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

	scope.Log("Uploaded %v (%v bytes)", output_path.AsClientPath(), offset)
	return &UploadResponse{
		Path:   output_path.AsClientPath(),
		Size:   uint64(offset),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}

func NewFileStoreUploader(
	config_obj *config_proto.Config,
	fs FileStore,
	root_path FSPathSpec) *FileStoreUploader {
	return &FileStoreUploader{fs, root_path}
}
