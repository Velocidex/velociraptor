package vql

import (
	"errors"
	"io"
	"os"
	"path"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/vfilter"
)

const TransferWellKnownFlowId = 5

// Returned as the result of the query.
type UploadResponse struct {
	Path  string
	Size  uint64
	Error string
}

// Provide an uploader capable of uploading any reader object.
type Uploader interface {
	Upload(scope *vfilter.Scope, filename string, reader io.Reader) (
		*UploadResponse, error)
}

type FileBasedUploader struct {
	UploadDir string
}

func (self *FileBasedUploader) Upload(
	scope *vfilter.Scope, filename string, reader io.Reader) (
	*UploadResponse, error) {
	if self.UploadDir == "" {
		scope.Log("UploadDir is not set")
		return nil, errors.New("UploadDir not set")
	}

	file_path := path.Join(self.UploadDir, filename)
	err := os.MkdirAll(path.Dir(file_path), 0700)
	if err != nil {
		scope.Log("Can not create dir: %s", err.Error())
		return nil, err
	}

	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		scope.Log("Unable to open file %s: %s", file_path, err.Error())
		return nil, err
	}
	defer file.Close()

	written, err := io.Copy(file, reader)
	if err != nil {
		scope.Log("Failed to copy file %s: %s", file_path, err.Error())
		return nil, err
	}
	scope.Log("Uploaded %v (%v bytes)", file_path, written)
	return &UploadResponse{
		Path: file_path,
		Size: uint64(written),
	}, nil
}

type VelociraptorUploader struct {
	Responder *responder.Responder
	Count     int
}

func (self *VelociraptorUploader) Upload(
	scope *vfilter.Scope, filename string, reader io.Reader) (
	*UploadResponse, error) {
	result := &UploadResponse{
		Path: filename,
	}

	offset := uint64(0)
	self.Count += 1
	for {
		buffer := make([]byte, 1024*1024)
		read_bytes, err := reader.Read(buffer)
		packet := &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				Path:     filename,
				Pathtype: actions_proto.PathSpec_OS,
			},
			Offset: offset,
			Data:   buffer[:read_bytes],
		}
		offset += uint64(read_bytes)

		if err != nil {
			// All done.
			if err == io.EOF {
				result.Size = offset
				return result, nil
			}
			// Other error - relay it back.
			return nil, err
		}

		// Send the packet to the server.
		self.Responder.AddResponseToRequest(
			TransferWellKnownFlowId, packet)
	}
}
