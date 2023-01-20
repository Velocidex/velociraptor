package reporting

import (
	"compress/flate"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alexmullins/zip"
	"github.com/go-errors/errors"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"

	"github.com/Velocidex/ordereddict"
	concurrent_zip "github.com/Velocidex/zip"
)

var (
	NO_METADATA         []vfilter.Row = nil
	DEFAULT_COMPRESSION int64         = 5
)

type ContainerFormat int

const (
	ContainerFormatJson       ContainerFormat = 1
	ContainerFormatCSV        ContainerFormat = 2
	ContainerFormatCSVAndJson ContainerFormat = ContainerFormatJson |
		ContainerFormatCSV
)

func GetContainerFormat(format string) (ContainerFormat, error) {
	switch format {
	case "json", "jsonl", "":
		return ContainerFormatJson, nil

	case "csv":
		return ContainerFormatCSVAndJson, nil

	case "csv_only":
		return ContainerFormatCSV, nil

	default:
	}
	return 0, fmt.Errorf(
		"Unknown format parameter %v either 'json', 'jsonl', 'cvs' or 'csv_only'.",
		format)
}

var (
	Clock utils.Clock = utils.RealClock{}
)

type MemberWriter struct {
	io.WriteCloser
	writer_wg *sync.WaitGroup

	owner *Container
}

func (self *MemberWriter) Write(buff []byte) (int, error) {
	self.owner.increaseUncompressedBytes(len(buff))
	return self.WriteCloser.Write(buff)
}

// Keep track of all members that are closed to allow the zip to be
// written properly.
func (self *MemberWriter) Close() error {
	err := self.WriteCloser.Close()
	self.writer_wg.Done()
	return err
}

type Container struct {
	config_obj *config_proto.Config

	// The underlying file writer
	fd io.WriteCloser

	// Calculate the hash of the final container.
	writer  *utils.TeeWriter
	sha_sum hash.Hash

	// Keep track of all the files we uploaded in the container itself.
	uploads []*uploads.UploadResponse

	level int

	// We write data to this zip file using the concurrent zip
	// implementation.
	zip *concurrent_zip.Writer

	// If a password is set, we create a new zip file here, and a
	// member within it then redirect the zip above to write on
	// it.
	delegate_zip *zip.Writer
	delegate_fd  io.Writer

	// manage orderly shutdown of the container.
	mu sync.Mutex

	// Keep stats about the container
	stats_mu sync.Mutex
	stats    api_proto.ContainerStats

	// Keep track of all writers so we can safely close the container.
	writer_wg sync.WaitGroup
	closed    bool
}

func (self *Container) Create(name string, mtime time.Time) (io.WriteCloser, error) {
	self.stats_mu.Lock()
	defer self.stats_mu.Unlock()
	self.stats.TotalContainerFiles++

	// Zip members must not be absolute
	name = strings.TrimPrefix(name, "/")

	self.writer_wg.Add(1)
	header := &concurrent_zip.FileHeader{
		Name:     name,
		Method:   concurrent_zip.Deflate,
		Modified: mtime,
	}

	if self.level == 0 {
		header.Method = concurrent_zip.Store
	}

	writer, err := self.zip.CreateHeader(header)
	if err != nil {
		return nil, err
	}

	return &MemberWriter{
		WriteCloser: writer,
		writer_wg:   &self.writer_wg,
		owner:       self,
	}, nil
}

func (self *Container) StoreArtifact(
	config_obj *config_proto.Config,
	ctx context.Context,
	scope vfilter.Scope,
	query *actions_proto.VQLRequest,
	prefix api.FSPathSpec,
	format ContainerFormat) (total_rows int, err error) {

	subctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Record query progress for stats and debugging.
	query_log := actions.QueryLog.AddQuery(query.VQL)
	defer query_log.Close()

	vql, err := vfilter.Parse(query.VQL)
	if err != nil {
		return 0, err
	}

	artifact_name := query.Name

	// Dont store un-named queries but run them anyway.
	if artifact_name == "" {
		for range vql.Eval(subctx, scope) {
			total_rows++
		}
		return total_rows, nil
	}

	// The name to use in the zip file to store results from this artifact
	dest := prefix.AddChild(artifact_name).AsClientPath()
	return self.WriteResultSet(subctx, config_obj, scope, format,
		dest, vql.Eval(subctx, scope))
}

func (self *Container) WriteResultSet(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope, format ContainerFormat,
	dest string, in <-chan vfilter.Row) (total_rows int, err error) {

	var result_set_writer *ContainerResultSetWriter

	// Only create JSON files if required.
	if format&ContainerFormatJson > 0 {
		result_set_writer, err = NewResultSetWriter(self, dest)
		if err != nil {
			return total_rows, err
		}

		defer func() {
			result_set_writer.Close()
		}()
	}

	// Optionally include CSV in the output
	var csv_writer *csv.CSVWriter
	if format&ContainerFormatCSV > 0 {
		csv_filename := strings.TrimSuffix(dest, ".json") + ".csv"
		csv_fd, err := self.Create(csv_filename, time.Time{})
		if err != nil {
			return total_rows, err
		}

		csv_writer = csv.GetCSVAppender(config_obj,
			scope, csv_fd,
			true, /* write_headers */
			json.NewEncOpts())

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
	for row := range in {
		total_rows++

		select {
		case <-ctx.Done():
			return

		default:
			if format&ContainerFormatJson > 0 {
				err := result_set_writer.Write(row)
				if err != nil {
					continue
				}
			}

			if csv_writer != nil {
				csv_writer.Write(row)
			}
		}
	}

	return total_rows, nil
}

func (self *Container) WriteJSON(name string, data interface{}) error {
	fd, err := self.Create(name, Clock.Now())
	if err != nil {
		return err
	}
	defer fd.Close()

	_, err = fd.Write(json.MustMarshalIndent(data))
	return err
}

// Parse the path according to the accessor to return the components
func (self *Container) getPathComponents(
	scope vfilter.Scope, accessor string, path string) ([]string, error) {

	file_store_factory, err := accessors.GetAccessor(accessor, scope)
	if err != nil {
		return nil, err
	}

	os_path, err := file_store_factory.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return os_path.Components, nil
}

func formatFilename(filename *accessors.OSPath, accessor string) string {
	result := filename.String()

	// These paths can be very large so we elide them.
	switch accessor {
	case "data", "sparse":
		if len(result) > 50 {
			result = result[:50] + "..."
		}
	}

	return result
}

// Upload the file into the zip file.  We treat the zip file as our
// filestore here - this means that file paths will be very
// conservatively escaped to ensure maximum compatibility with zip
// programs. We store the proper paths and the StoredName in the
// uploads.json file for easy retrieval later.
func (self *Container) Upload(
	ctx context.Context,
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor string,
	store_as_name *accessors.OSPath,
	expected_size int64,
	mtime time.Time,
	atime time.Time,
	ctime time.Time,
	btime time.Time,
	reader io.Reader) (*uploads.UploadResponse, error) {

	result := &uploads.UploadResponse{
		Path: formatFilename(filename, accessor),
		Size: uint64(expected_size),
	}

	// Avoid sending huge strings inside the JSON
	if accessor == "data" {
		result.Path = "data"
	}

	// The filename to store the file inside the zip - due to escaping
	// issues this may not be exactly the same as the file name we
	// receive.
	if store_as_name == nil {
		store_as_name = filename
	}

	store_path, err := accessors.NewZipFilePath("uploads")
	if err != nil {
		return nil, err
	}

	store_path = store_path.Append(accessor).
		Append(store_as_name.Components...)

	// Where to store the file inside the Zip file.
	result.StoredName = store_path.String()
	result.Components = store_path.Components

	scope.Log("Collecting file %s into %s (%v bytes)",
		formatFilename(filename, accessor), result.StoredName, expected_size)

	// Try to collect sparse files if possible
	err = self.maybeCollectSparseFile(ctx, scope, reader, result, mtime)
	if err == nil {
		self.mu.Lock()
		self.uploads = append(self.uploads, result)
		self.mu.Unlock()

		return result, nil
	}

	writer, err := self.Create(result.StoredName, mtime)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	sha_sum := sha256.New()
	md5_sum := md5.New()

	count, err := utils.Copy(ctx, utils.NewTee(writer, sha_sum, md5_sum), reader)
	if err != nil {
		result.StoredSize = uint64(count)
		result.Error = err.Error()
		return result, err
	}

	result.StoredSize = uint64(count)
	result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
	result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))

	self.mu.Lock()
	self.uploads = append(self.uploads, result)
	self.mu.Unlock()

	self.stats_mu.Lock()
	self.stats.TotalUploadedBytes += result.Size
	self.stats_mu.Unlock()

	return result, nil
}

func (self *Container) maybeCollectSparseFile(
	ctx context.Context,
	scope vfilter.Scope,
	reader io.Reader,
	result *uploads.UploadResponse, mtime time.Time) error {

	// Can the reader produce ranges?
	range_reader, ok := reader.(uploads.RangeReader)
	if !ok {
		return errors.New("Not supported")
	}

	writer, err := self.Create(result.StoredName, mtime)
	if err != nil {
		return err
	}
	defer writer.Close()

	sha_sum := sha256.New()
	md5_sum := md5.New()

	// The byte count we write to the output file.
	count := 0

	// An index array for sparse files.
	index := &actions_proto.Index{}
	is_sparse := false

	for _, rng := range range_reader.Ranges() {
		file_length := rng.Length
		if rng.IsSparse {
			file_length = 0
		}

		index.Ranges = append(index.Ranges,
			&actions_proto.Range{
				FileOffset:     int64(count),
				OriginalOffset: rng.Offset,
				FileLength:     file_length,
				Length:         rng.Length,
			})

		if rng.IsSparse {
			is_sparse = true
			continue
		}

		_, err = range_reader.Seek(rng.Offset, io.SeekStart)
		if err != nil {
			return err
		}

		run_writer := utils.NewTee(writer, sha_sum, md5_sum)
		n, err := utils.CopyN(ctx, run_writer, range_reader, rng.Length)
		if err != nil {
			return err
		}

		// We were unable to fully copy this run - this could indicate
		// an issue with decompression of the ntfs for
		// example. However we still need to maintain alignment here
		// so we pad with zeros.
		if int64(n) < rng.Length {
			scope.Log("Unable to fully copy range %v in %v - padding %v bytes",
				rng, result.StoredName, rng.Length-int64(n))
			_, _ = utils.CopyN(
				ctx, run_writer, utils.ZeroReader{}, rng.Length-int64(n))
		}

		count += n
	}

	// If there were any sparse runs, create an index.
	if is_sparse {
		idx_upload := &uploads.UploadResponse{
			Components: utils.CopySlice(result.Components),
			StoredName: result.StoredName + ".idx",
			Path:       result.Path + ".idx",
			Reference:  result.Reference,
			Type:       "idx",
		}
		writer, err := self.Create(idx_upload.StoredName, time.Time{})
		if err != nil {
			return err
		}
		defer writer.Close()

		serialized, err := json.Marshal(index)
		if err != nil {
			return err
		}

		_, err = writer.Write(serialized)
		if err != nil {
			return err
		}

		idx_upload.Size = uint64(len(serialized))
		idx_upload.StoredSize = uint64(len(serialized))

		// Add it to the uploads list.
		self.mu.Lock()
		self.uploads = append(self.uploads, idx_upload)
		self.mu.Unlock()
	}

	result.StoredSize = uint64(count)
	result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
	result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))
	return nil
}

func (self *Container) IsClosed() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.closed
}

// Close the underlying container zip (and write central
// directories). It is ok to call this multiple times.
func (self *Container) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.closed {
		return nil
	}
	self.closed = true

	if len(self.uploads) > 0 {
		result_set_writer, err := NewResultSetWriter(self, "uploads.json")
		if err == nil {
			for _, record := range self.uploads {
				err = result_set_writer.Write(ordereddict.NewDict().
					Set("Timestamp", Clock.Now()).
					Set("started", Clock.Now().UTC().String()).
					Set("vfs_path", record.Path).
					Set("_Components", record.Components).
					Set("file_size", record.Size).
					Set("uploaded_size", record.StoredSize).
					Set("Type", record.Type))
				if err != nil {
					break
				}
			}
			result_set_writer.Close()
		}
	}

	// Wait for all outstanding writers to finish before we close the
	// zip file.
	self.writer_wg.Wait()

	self.zip.Close()

	if self.delegate_zip != nil {
		self.delegate_zip.Close()
	}

	// Only report the hash if we actually wrote something (few bytes
	// are always written for the zip header).
	hash := hex.EncodeToString(self.sha_sum.Sum(nil))
	self.stats_mu.Lock()
	self.stats.Hash = hash
	self.stats_mu.Unlock()

	if self.writer.Count() > 50 {
		logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
		logger.Info("Container hash %v", hash)

	}
	return self.fd.Close()
}

func (self *Container) increaseUncompressedBytes(len int) {
	self.stats_mu.Lock()
	defer self.stats_mu.Unlock()

	self.stats.TotalUncompressedBytes += uint64(len)
}

func (self *Container) Stats() *api_proto.ContainerStats {
	self.stats_mu.Lock()
	// Take a copy
	stats := proto.Clone(&self.stats).(*api_proto.ContainerStats)
	self.stats_mu.Unlock()

	stats.TotalUploadedFiles = uint64(len(self.uploads))
	stats.TotalCompressedBytes = uint64(self.writer.Count())
	stats.TotalDuration = uint64(Clock.Now().Unix()) - stats.Timestamp

	return stats
}

func NewContainer(
	config_obj *config_proto.Config,
	path string, password string, level int64, metadata []vfilter.Row) (*Container, error) {
	fd, err := os.OpenFile(
		path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}

	return NewContainerFromWriter(config_obj, fd, password, level, metadata)
}

func NewContainerFromWriter(
	config_obj *config_proto.Config, fd io.WriteCloser,
	password string, level int64, metadata []vfilter.Row) (*Container, error) {

	var err error

	if level < 0 || level > 9 {
		level = 5
	}

	sha_sum := sha256.New()

	result := &Container{
		config_obj: config_obj,
		fd:         fd,
		sha_sum:    sha_sum,
		writer:     utils.NewTee(fd, sha_sum),
		level:      int(level),
	}

	result.stats.Timestamp = uint64(Clock.Now().Unix())

	// We need to build a protected container.
	if password != "" {

		result.delegate_zip = zip.NewWriter(result.writer)
		if metadata != nil && len(metadata) != 0 {
			fh, err := result.delegate_zip.Create("metadata.json")
			if err != nil {
				return nil, err
			}
			fh.Write(json.MustMarshalIndent(metadata))
		}
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
		result.zip = concurrent_zip.NewWriter(result.writer)
		result.zip.RegisterCompressor(
			zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
				return flate.NewWriter(out, int(level))
			})
		if metadata != nil && len(metadata) != 0 {
			fh, err := result.zip.Create("metadata.json")
			if err != nil {
				return nil, err
			}
			fh.Write(json.MustMarshalIndent(metadata))
			fh.Close()

		}
	}

	return result, nil
}
