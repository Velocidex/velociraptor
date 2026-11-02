package remote

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FileWriter struct {
	filename   api.FSPathSpec
	size       int64
	truncate   bool
	ctx        context.Context
	config_obj *config_proto.Config

	completion func()
}

// FIXME: Should we handle file locks for non-concurrent writers?
func (self *FileWriter) Size() (int64, error) {
	return self.size, nil
}

// File writes are always append, unless we need to truncate it
// first. So regular file writes do not carry an offset.
func (self *FileWriter) Write(data []byte) (int, error) {
	req := &api_proto.VFSFileBuffer{
		Data: data,
	}
	if self.truncate {
		req.Truncate = true
		self.truncate = false
	}
	return self.writeMsg(req)
}

func (self *FileWriter) WriteCompressed(data []byte,
	offset uint64, uncompressed_size int) (int, error) {
	req := &api_proto.VFSFileBuffer{
		Data:             data,
		UncompressedSize: uint64(uncompressed_size),
		Offset:           offset,
	}
	if self.truncate {
		req.Truncate = true
		self.truncate = false
	}
	return self.writeMsg(req)
}

func (self *FileWriter) writeMsg(req *api_proto.VFSFileBuffer) (int, error) {
	req.Components = self.filename.Components()
	req.PathType = int64(self.filename.Type())
	req.OrgId = self.config_obj.OrgId

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

	grpc_result, err := conn.FSWriteBuffer(ctx, req)
	if err != nil {
		return 0, utils.StatusToError(err)
	}

	self.size += grpc_result.Written

	return int(grpc_result.Written), utils.GetError(grpc_result.Error)
}

func (self *FileWriter) Truncate() error {
	self.truncate = true
	_, err := self.Write([]byte{})
	return err
}

func (self *FileWriter) Close() error {
	if self.completion != nil {
		self.completion()
	}
	return nil
}

func (self *FileWriter) Update(data []byte, offset int64) error {
	return utils.Wrap(utils.NotImplementedError, "FileWriter.Update")
}

func (self *FileWriter) Flush() error {
	return nil
}

func (self *RPCFileStore) WriteFile(
	filename api.FSPathSpec) (api.FileWriter, error) {
	return self.WriteFileWithCompletion(filename, func() {})
}

func (self *RPCFileStore) WriteFileWithCompletion(
	filename api.FSPathSpec,
	completion func()) (api.FileWriter, error) {
	return &FileWriter{
		filename:   filename,
		ctx:        self.ctx,
		config_obj: self.config_obj,
	}, nil
}
