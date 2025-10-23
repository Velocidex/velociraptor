package networking

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	BoundaryForTests = ""
)

type fileSpec struct {
	File     string            `vfilter:"required,field=file,doc=The form name of the file to upload"`
	Key      string            `vfilter:"required,field=key,doc=The form key of the file to upload (e.g. 'file')"`
	Path     *accessors.OSPath `vfilter:"required,field=path,doc=A path to the file to upload"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`

	reader  io.Reader
	size    int64
	closer  func() error
	headers []byte
}

// Builds a multipart reader that does not buffer into memory. Allows
// uploading massive files.
func GetMultiPartReader(
	ctx context.Context,
	scope vfilter.Scope,
	files []*ordereddict.Dict,
	params *ordereddict.Dict) (*multiPartReader, error) {

	result := &multiPartReader{
		parameters_buffer: &bytes.Buffer{},
	}

	// Open all the files and check they are seekable.
	for _, file := range files {
		file_spec := &fileSpec{}
		err := arg_parser.ExtractArgsWithContext(
			ctx, scope, file, file_spec)
		if err != nil {
			return nil, err
		}

		// Check the user's access to this file.
		accessor, err := accessors.GetAccessor(file_spec.Accessor, scope)
		if err != nil {
			scope.Log("http_client: When uploading %v: %v",
				file_spec.Path.String(), err)
			continue
		}

		fd, err := accessor.OpenWithOSPath(file_spec.Path)
		if err != nil {
			scope.Log("http_client: When uploading %v: %v",
				file_spec.Path.String(), err)
			continue
		}

		stat, err := accessor.LstatWithOSPath(file_spec.Path)
		if err != nil {
			scope.Log("http_client: File must be seekable %v: %v",
				file_spec.Path.String(), err)
			continue
		}

		file_spec.reader = fd
		file_spec.size = stat.Size()
		file_spec.closer = fd.Close

		file_spec.headers = append(file_spec.headers,
			[]byte(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\nContent-Type: application/octet-stream\r\n\r\n",
				escapeQuotes(file_spec.Key), escapeQuotes(file_spec.File)))...)
		result.files = append(result.files, file_spec)
	}

	// Dump params into memory so we can figure out the size.
	result.multipart_writer = multipart.NewWriter(
		result.parameters_buffer)
	if BoundaryForTests != "" {
		err := result.multipart_writer.SetBoundary(BoundaryForTests)
		if err != nil {
			return nil, err
		}
	}

	// Encode any parameters into the form first
	if params != nil {
		for _, i := range params.Items() {
			err := result.multipart_writer.WriteField(
				i.Key, utils.ToString(i.Value))
			if err != nil {
				return nil, err
			}
		}
	}

	result.pipe_reader, result.pipe_writer = io.Pipe()
	result.openBoundary = []byte(fmt.Sprintf("\r\n--%s\r\n",
		result.multipart_writer.Boundary()))
	result.closeBoundary = []byte(fmt.Sprintf("\r\n--%s--\r\n",
		result.multipart_writer.Boundary()))

	// Start streaming output
	go func() {
		_ = result.Start()
	}()

	return result, nil
}

type multiPartReader struct {
	pipe_reader *io.PipeReader
	pipe_writer *io.PipeWriter

	parameters_buffer *bytes.Buffer
	files             []*fileSpec

	multipart_writer *multipart.Writer

	openBoundary  []byte
	closeBoundary []byte
}

func (self *multiPartReader) ContentLength() int {
	// All the parameters first
	length := len(self.parameters_buffer.Bytes())

	// Now calculate the len of each file.
	for _, file_spec := range self.files {
		// First the openBoundary
		length += len(self.openBoundary)

		// Now the headers
		length += len(file_spec.headers)

		// Now the file size
		length += int(file_spec.size)
	}

	// Finally the end boundary
	length += len(self.closeBoundary)

	return length
}

func (self *multiPartReader) ContentType() string {
	return "multipart/form-data; boundary=" +
		self.multipart_writer.Boundary()
}

func (self *multiPartReader) Reader() io.Reader {
	return self.pipe_reader
}

func (self *multiPartReader) Debug() string {
	result, err := utils.ReadAllWithLimit(self.pipe_reader,
		constants.MAX_MEMORY)
	if err != nil {
		return fmt.Sprintf("Error: %v\n", err)
	}
	return string(result)
}

// Start streaming the data
func (self *multiPartReader) Start() error {
	defer self.pipe_writer.Close()

	// Dump the params out from our memory buffer
	_, err := self.pipe_writer.Write(self.parameters_buffer.Bytes())
	if err != nil {
		return err
	}

	// Now push each file out
	for _, file_info := range self.files {
		// First boundary
		_, err = self.pipe_writer.Write(self.openBoundary)
		if err != nil {
			return err
		}

		// Then headers
		_, err = self.pipe_writer.Write(file_info.headers)
		if err != nil {
			return err
		}

		// Now copy the file over.
		_, err = io.Copy(self.pipe_writer, file_info.reader)
		if err != nil {
			return err
		}

		// Now close the file.
		err = file_info.closer()
		if err != nil {
			return err
		}
	}

	// Write the end boundary
	_, err = self.pipe_writer.Write(self.closeBoundary)

	return err
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}
