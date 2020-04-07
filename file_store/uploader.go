package file_store

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/vfilter"
)

// An uploader into the filestore.
type FileStoreUploader struct {
	file_store FileStore
	root_path  string
}

func (self *FileStoreUploader) Upload(
	ctx context.Context,
	scope *vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	expected_size int64,
	reader io.Reader) (
	*uploads.UploadResponse, error) {

	if store_as_name == "" {
		store_as_name = filename
	}

	output_path := path.Join(self.root_path, store_as_name)

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

	for {
		n, _ := reader.Read(buf)
		if n == 0 {
			break
		}
		data := buf[:n]

		out_fd.Write(data)
		md5_sum.Write(data)
		sha_sum.Write(data)

		offset += int64(n)
	}

	scope.Log("Uploaded %v (%v bytes)", output_path, offset)
	return &uploads.UploadResponse{
		Path:   output_path,
		Size:   uint64(offset),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}

func NewFileStoreUploader(
	config_obj *config_proto.Config,
	root_path string) *FileStoreUploader {

	fs := GetFileStore(config_obj)
	return &FileStoreUploader{fs, root_path}
}
