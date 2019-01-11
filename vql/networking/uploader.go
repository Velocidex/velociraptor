package networking

import (
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// Returned as the result of the query.
type UploadResponse struct {
	Path  string
	Size  uint64
	Error string
}

// Provide an uploader capable of uploading any reader object.
type Uploader interface {
	Upload(scope *vfilter.Scope,
		filename string,
		accessor string,
		store_as_name string,
		reader io.Reader) (*UploadResponse, error)
}

type FileBasedUploader struct {
	UploadDir string
}

// Turn the path which may have a device name into something which can
// be created as a directory.
var sanitize_re = regexp.MustCompile(`[^a-zA-Z0-9_@\(\)\. \-=\{\}\[\]]`)

func sanitize_path(path string) string {
	// Strip any leading devices, and make sure the device name
	// consists of valid chars.
	path = regexp.MustCompile(
		`\\\\[\\.\\?]\\([{}a-zA-Z0-9]+).*?\\`).
		ReplaceAllString(path, `$1\`)

	// Split the path into components and escape any non valid
	// chars.
	res := []string{}
	components := utils.SplitComponents(path)
	for _, component := range components {
		if len(component) > 0 {
			res = append(
				res,
				sanitize_re.ReplaceAllStringFunc(
					component, func(x string) string {
						result := make([]byte, 2)
						hex.Encode(result, []byte(x))
						return "%" + string(result)
					}))
		}
	}

	result := strings.Join(res, "/")
	return result
}

func (self *FileBasedUploader) Upload(
	scope *vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	reader io.Reader) (
	*UploadResponse, error) {
	if self.UploadDir == "" {
		scope.Log("UploadDir is not set")
		return nil, errors.New("UploadDir not set")
	}

	if store_as_name == "" {
		store_as_name = sanitize_path(filename)
	}

	file_path := filepath.Join(self.UploadDir, store_as_name)
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
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

	buf := make([]byte, 1024*1024)
	offset := int64(0)
	for {
		n, _ := reader.Read(buf)
		if n == 0 {
			break
		}
		file.Write(buf[:n])
		offset += int64(n)
	}

	scope.Log("Uploaded %v (%v bytes)", file_path, offset)
	return &UploadResponse{
		Path: file_path,
		Size: uint64(offset),
	}, nil
}

type VelociraptorUploader struct {
	Responder *responder.Responder
	Count     int
}

func (self *VelociraptorUploader) Upload(
	scope *vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	reader io.Reader) (
	*UploadResponse, error) {
	result := &UploadResponse{
		Path: filename,
	}

	if store_as_name == "" {
		store_as_name = filename
	}

	offset := uint64(0)
	self.Count += 1
	buffer := make([]byte, 1024*1024)

	for {
		read_bytes, err := reader.Read(buffer)
		if read_bytes == 0 {
			result.Size = offset
			return result, nil
		}

		packet := &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				Path:     store_as_name,
				Accessor: accessor,
			},
			Offset: offset,
			Data:   buffer[:read_bytes],
		}
		// Send the packet to the server.
		self.Responder.AddResponseToRequest(
			constants.TransferWellKnownFlowId, packet)

		offset += uint64(read_bytes)
		if err != nil && err != io.EOF {
			return nil, err
		}
	}
}
