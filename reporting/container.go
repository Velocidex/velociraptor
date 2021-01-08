package reporting

import (
	"compress/flate"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/alexmullins/zip"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"

	concurrent_zip "github.com/Velocidex/zip"
)

type Container struct {
	// The underlying file writer
	fd io.WriteCloser

	level int

	// We write data to this zip file using the concurrent zip
	// implementation.
	zip *concurrent_zip.Writer

	// If a password is set, we create a new zip file here, and a
	// member within it then redirect the zip above to write on
	// it.
	delegate_zip *zip.Writer
	delegate_fd  io.Writer
}

func (self *Container) Create(name string) (io.WriteCloser, error) {
	header := &concurrent_zip.FileHeader{
		Name:   name,
		Method: concurrent_zip.Deflate,
	}

	if self.level == 0 {
		header.Method = concurrent_zip.Store
	}

	return self.zip.CreateHeader(header)
}

func (self *Container) StoreArtifact(
	config_obj *config_proto.Config,
	ctx context.Context,
	scope vfilter.Scope,
	query *actions_proto.VQLRequest,
	format string) (err error) {

	vql, err := vfilter.Parse(query.VQL)
	if err != nil {
		return err
	}

	artifact_name := query.Name

	// Dont store un-named queries but run them anyway.
	if artifact_name == "" {
		for range vql.Eval(ctx, scope) {
		}
		return nil
	}

	// The name to use in the zip file to store results from this artifact
	path_manager := NewContainerPathManager(artifact_name)
	fd, err := self.Create(path_manager.Path())
	if err != nil {
		return err
	}

	// Preserve the error for our caller.
	defer func() {
		err_ := fd.Close()
		if err == nil {
			err = err_
		}
	}()

	// Optionally include CSV in the output
	var csv_writer *csv.CSVWriter
	if format == "csv" {
		csv_fd, err := self.Create(path_manager.CSVPath())
		if err != nil {
			return err
		}

		csv_writer = csv.GetCSVAppender(
			scope, csv_fd, true /* write_headers */)

		// Preserve the error for our caller.
		defer func() {
			csv_writer.Close()
			err_ := csv_fd.Close()
			if err == nil {
				err = err_
			}
		}()
	}

	// Store as line delimited JSON
	marshaler := vql_subsystem.MarshalJsonl(scope)
	for row := range vql.Eval(ctx, scope) {
		// Re-serialize it as compact json.
		serialized, err := marshaler([]vfilter.Row{row})
		if err != nil {
			continue
		}

		_, err = fd.Write(serialized)
		if err != nil {
			return errors.WithStack(err)
		}

		if csv_writer != nil {
			csv_writer.Write(row)
		}
	}

	return nil
}

func sanitize_upload_name(store_as_name string) string {
	components := []string{}
	// Normalize and clean up the path so the zip file is more
	// usable by fragile zip programs like Windows explorer.
	for _, component := range utils.SplitComponents(store_as_name) {
		if component == "." || component == ".." {
			continue
		}
		components = append(components, sanitize(component))
	}

	// Zip members must not have absolute paths.
	return path.Join(components...)
}

func sanitize(component string) string {
	component = strings.Replace(component, ":", "", -1)
	component = strings.Replace(component, "?", "", -1)
	return component
}

func (self *Container) Upload(
	ctx context.Context,
	scope vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	expected_size int64,
	reader io.Reader) (*api.UploadResponse, error) {

	if store_as_name == "" {
		store_as_name = accessor + "/" + filename
	}

	sanitized_name := sanitize_upload_name(store_as_name)

	scope.Log("Collecting file %s into %s (%v bytes)",
		filename, store_as_name, expected_size)

	// Try to collect sparse files if possible
	result, err := self.maybeCollectSparseFile(
		ctx, reader, store_as_name, sanitized_name)
	if err == nil {
		return result, nil
	}

	writer, err := self.Create(sanitized_name)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	sha_sum := sha256.New()
	md5_sum := md5.New()

	n, err := utils.Copy(ctx, utils.NewTee(writer, sha_sum, md5_sum), reader)
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	return &api.UploadResponse{
		Path:   sanitized_name,
		Size:   uint64(n),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}

func (self *Container) maybeCollectSparseFile(
	ctx context.Context,
	reader io.Reader, store_as_name, sanitized_name string) (
	*api.UploadResponse, error) {

	// Can the reader produce ranges?
	range_reader, ok := reader.(uploads.RangeReader)
	if !ok {
		return nil, errors.New("Not supported")
	}

	writer, err := self.Create(sanitized_name)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	sha_sum := sha256.New()
	md5_sum := md5.New()

	// The byte count we write to the output file.
	count := 0

	// An index array for sparse files.
	index := []*ordereddict.Dict{}
	is_sparse := false

	for _, rng := range range_reader.Ranges() {
		file_length := rng.Length
		if rng.IsSparse {
			file_length = 0
		}

		index = append(index, ordereddict.NewDict().
			Set("file_offset", count).
			Set("original_offset", rng.Offset).
			Set("file_length", file_length).
			Set("length", rng.Length))

		if rng.IsSparse {
			is_sparse = true
			continue
		}

		_, err = range_reader.Seek(rng.Offset, io.SeekStart)
		if err != nil {
			return &api.UploadResponse{
				Error: err.Error(),
			}, err
		}

		n, err := utils.CopyN(ctx, utils.NewTee(writer, sha_sum, md5_sum),
			range_reader, rng.Length)
		if err != nil {
			return &api.UploadResponse{
				Error: err.Error(),
			}, err
		}
		count += n
	}

	// If there were any sparse runs, create an index.
	if is_sparse {
		writer, err := self.Create(sanitized_name + ".idx")
		if err != nil {
			return nil, err
		}
		defer writer.Close()

		serialized, err := utils.DictsToJson(index, nil)
		if err != nil {
			return &api.UploadResponse{
				Error: err.Error(),
			}, err
		}

		_, err = writer.Write(serialized)
		if err != nil {
			return &api.UploadResponse{
				Error: err.Error(),
			}, err
		}
	}

	return &api.UploadResponse{
		Path:   sanitized_name,
		Size:   uint64(count),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}

func (self *Container) Close() error {
	self.zip.Close()

	if self.delegate_zip != nil {
		self.delegate_zip.Close()
	}
	return self.fd.Close()
}

func NewContainer(path string, password string, level int64) (*Container, error) {
	fd, err := os.OpenFile(
		path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}

	if level < 0 || level > 9 {
		level = 5
	}

	result := &Container{
		fd:    fd,
		level: int(level),
	}

	// We need to build a protected container.
	if password != "" {
		result.delegate_zip = zip.NewWriter(fd)

		// We are writing a zip file into here - no need to
		// compress.
		fh := &zip.FileHeader{
			Name:   "data.zip",
			Method: zip.Store,
		}
		fh.SetPassword(password)
		result.delegate_fd, err = result.delegate_zip.CreateHeader(fh)
		if err != nil {
			return nil, err
		}

		result.zip = concurrent_zip.NewWriter(result.delegate_fd)
	} else {
		result.zip = concurrent_zip.NewWriter(result.fd)
		result.zip.RegisterCompressor(
			zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
				return flate.NewWriter(out, int(level))
			})
	}

	return result, nil
}

// Turns os.Stdout into into file_store.WriteSeekCloser
type StdoutWrapper struct {
	io.Writer
}

func (self *StdoutWrapper) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (self *StdoutWrapper) Close() error {
	return nil
}

func (self *StdoutWrapper) Truncate(offset int64) error {
	return nil
}
