package reporting

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type NotebookUploader struct {
	config_obj  *config_proto.Config
	PathManager *paths.NotebookCellPathManager
}

func (self *NotebookUploader) Upload(
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
	reader io.Reader) (
	*uploads.UploadResponse, error) {

	if store_as_name == "" {
		store_as_name = filename.String()
	}
	dest_path_spec := self.PathManager.GetUploadsFile(store_as_name)

	file_store_factory := file_store.GetFileStore(self.config_obj)
	writer, err := file_store_factory.WriteFile(dest_path_spec)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	md5_sum := md5.New()
	sha_sum := sha256.New()

	n, err := utils.Copy(ctx, writer, io.TeeReader(
		io.TeeReader(reader, sha_sum), md5_sum))
	return &uploads.UploadResponse{
		Path:   store_as_name,
		Size:   uint64(n),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, err
}
