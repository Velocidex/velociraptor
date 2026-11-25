package remote

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/alitto/pond/v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type FileReader struct {
	filename   api.FSPathSpec
	offset     int64
	ctx        context.Context
	config_obj *config_proto.Config

	factory func() (api_proto.APIClient, func() error, error)
}

func (self *FileReader) Read(buff []byte) (int, error) {
	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(self.config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := self.factory()
	if err != nil {
		return 0, err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	// split the buffer into smaller rpc sized buffers.
	pool := pond.NewPool(10, pond.WithContext(ctx))
	total_read := 0
	var mu sync.Mutex

	// The largest buffer we can transfer in one call
	var buf_size int64 = constants.GRPC_DEFAULT_TRANSFER_SIZE
	if self.config_obj.ApiConfig != nil &&
		self.config_obj.ApiConfig.MaxGrpcRecvSize > 2*10240 {
		buf_size = int64(self.config_obj.ApiConfig.MaxGrpcRecvSize)
	}

	// Leave a bit of space for other fields.
	buf_size -= 10240

	var tasks []pond.Task

	for offset := int64(0); offset < int64(len(buff)); offset += buf_size {
		tasks = append(tasks, pool.SubmitErr(func() error {
			to_read := buf_size
			end_of_buff := int64(len(buff))

			if offset+to_read > end_of_buff {
				to_read = end_of_buff - offset
			}

			grpc_result, err := conn.VFSGetBuffer(ctx,
				&api_proto.VFSFileBuffer{
					Offset:     uint64(offset + self.offset),
					Length:     uint32(to_read),
					Components: self.filename.Components(),
					PathType:   int64(self.filename.Type()),
					OrgId:      self.config_obj.OrgId,
				})

			if err != nil {
				return utils.StatusToError(err)
			}

			if grpc_result.Data == nil || len(grpc_result.Data) == 0 {
				return io.EOF
			}

			if to_read > int64(len(grpc_result.Data)) {
				to_read = int64(len(grpc_result.Data))
			}

			// Copy the data into the buffer
			mu.Lock()
			for i := int64(0); i < to_read; i++ {
				buff[i+offset] = grpc_result.Data[i]
			}
			total_read += int(to_read)
			mu.Unlock()

			// A short read must be at the end of file.
			if to_read < buf_size {
				err = io.EOF
			}

			return err
		}))
	}

	// Wait here for all the calls
	for _, t := range tasks {
		err1 := t.Wait()
		if err == nil {
			err = err1
		}
	}

	self.offset += int64(total_read)

	return total_read, err
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
	default:
		return 0, utils.InvalidArgError
	}

	return self.offset, nil
}

func (self *FileReader) Stat() (api.FileInfo, error) {
	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(self.config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := self.factory()
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

	urn := path_specs.NewUnsafeFilestorePath(grpc_result.Components...).
		SetType(api.PathType(grpc_result.Type))

	return &vtesting.MockFileInfo{
		Name_:     urn.Base(),
		PathSpec_: urn,
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
		factory:    self.factory,
	}, nil
}
