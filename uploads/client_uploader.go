package uploads

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/vfilter"
)

// An uploader delivering files from client to server.
type VelociraptorUploader struct {
	Responder *responder.Responder
	Count     int
}

func (self *VelociraptorUploader) Upload(
	ctx context.Context,
	scope *vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	expected_size int64,
	reader io.Reader) (
	*api.UploadResponse, error) {
	result := &api.UploadResponse{
		Path: filename,
	}

	if store_as_name == "" {
		store_as_name = filename
	}

	offset := uint64(0)
	self.Count += 1

	md5_sum := md5.New()
	sha_sum := sha256.New()

	for {
		// Ensure there is a fresh allocation for every
		// iteration to prevent overwriting in flight buffers.
		buffer := make([]byte, 1024*1024)
		read_bytes, err := reader.Read(buffer)
		data := buffer[:read_bytes]
		sha_sum.Write(data)
		md5_sum.Write(data)

		packet := &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				Path:     store_as_name,
				Accessor: accessor,
			},
			Offset: offset,
			Size:   uint64(expected_size),
			Data:   data,
			Eof:    err == io.EOF,
		}

		select {
		case <-ctx.Done():
			return nil, errors.New("Cancelled!")

		default:
			// Send the packet to the server.
			self.Responder.AddResponse(&crypto_proto.GrrMessage{
				RequestId:  constants.TransferWellKnownFlowId,
				FileBuffer: packet})
		}

		offset += uint64(read_bytes)
		if err != nil && err != io.EOF {
			return nil, err
		}

		if read_bytes == 0 {
			result.Size = offset
			result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
			result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))
			return result, nil
		}
	}
}
