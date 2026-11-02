package remote

import (
	"context"
	"fmt"
	"io"
	"os"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type FileReader struct {
	filename   api.FSPathSpec
	offset     int64
	ctx        context.Context
	config_obj *config_proto.Config
}

func (self *FileReader) Read(buff []byte) (int, error) {
	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(self.config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx,
		grpc_client.SuperUser, self.config_obj)
	if err != nil {
		return 0, err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	fmt.Printf("Read %v from %v\n", len(buff), self.offset)

	grpc_result, err := conn.VFSGetBuffer(ctx,
		&api_proto.VFSFileBuffer{
			Offset:     uint64(self.offset),
			Length:     uint32(len(buff)),
			Components: self.filename.Components(),
			PathType:   int64(self.filename.Type()),
			OrgId:      self.config_obj.OrgId,
		})
	if err != nil {
		return 0, utils.StatusToError(err)
	}

	if grpc_result.Data == nil {
		return 0, io.EOF
	}

	to_read := len(buff)
	if to_read > len(grpc_result.Data) {
		to_read = len(grpc_result.Data)
	}

	for i := 0; i < to_read; i++ {
		buff[i] = grpc_result.Data[i]
	}

	// A short read must be at the end of file.
	if to_read < len(buff) {
		err = io.EOF
	}

	self.offset += int64(to_read)

	return to_read, err
}

func (self *FileReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case os.SEEK_CUR:
		self.offset += offset
	case os.SEEK_SET:
		self.offset = offset
	case os.SEEK_END:
		s, err := self.Stat()
		if err != nil {
			return 0, err
		}
		self.offset = s.Size() + offset
	}
	return 0, utils.InvalidArgError
}

func (self *FileReader) Stat() (api.FileInfo, error) {
	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(self.config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx,
		grpc_client.SuperUser, self.config_obj)
	if err != nil {
		return nil, err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	grpc_result, err := conn.FSStat(ctx,
		&api_proto.DataRequest{
			Pathspec: &api_proto.DSPathSpec{
				Components: self.filename.Components(),
				PathType:   int64(self.filename.Type()),
			},
			OrgId: self.config_obj.OrgId,
		})
	if err != nil {
		return nil, utils.StatusToError(err)
	}

	urn := path_specs.NewUnsafeDatastorePath(grpc_result.Components...).
		SetType(api.PathType(grpc_result.Type))

	return &vtesting.MockFileInfo{
		Name_:     urn.Base(),
		PathSpec_: urn.AsFilestorePath(),
		FullPath_: urn.String(),
		Mode_:     os.FileMode(grpc_result.Mode),
		Size_:     int64(grpc_result.Size),
	}, nil
}

func (self *FileReader) Close() error {
	return nil
}

func (self *RPCFileStore) ReadFile(filename api.FSPathSpec) (api.FileReader, error) {
	return &FileReader{
		filename:   filename,
		ctx:        self.ctx,
		config_obj: self.config_obj,
	}, nil
}
