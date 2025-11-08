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
	"unicode"

	"github.com/alexmullins/zip"
	"github.com/dustin/go-humanize"
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
	"www.velocidex.com/golang/velociraptor/utils/files"
	"www.velocidex.com/golang/vfilter"

	"github.com/Velocidex/ordereddict"
	concurrent_zip "github.com/Velocidex/zip"
)

var (
	NO_METADATA         []vfilter.Row = nil
	DEFAULT_COMPRESSION int64         = 5

	ZipRootPath = accessors.MustNewZipFilePath("/")

	pool = sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, 1024*1024)
			return &buffer
		},
	}
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
		"Unknown format parameter %v either 'json', 'jsonl', 'csv' or 'csv_only'.",
		format)
}

var (
	Clock utils.Clock = utils.RealClock{}
)

type MemberWriter struct {
	io.WriteCloser
	writer_wg *sync.WaitGroup

	owner *Container

	stats_provider concurrent_zip.StatsWriter
	id             uint64
}

func (self *MemberWriter) Write(buff []byte) (int, error) {
	self.owner.increaseUncompressedBytes(len(buff))
	res, err := self.WriteCloser.Write(buff)

	ContainerTracker.UpdateContainerWriter(self.owner.id, self.id,
		func(info *WriterInfo) {
			if self.stats_provider != nil {
				stats := self.stats_provider.GetStats()
				info.CompressedSize = int(stats.CompressedSize)
				info.TmpFile = stats.TmpFile
			}
			info.UncompressedSize += res
			info.LastWrite = utils.GetTime().Now()
		})

	// FIXME: Use this to instrument a very slow export
	// time.Sleep(200 * time.Millisecond)

	return res, err
}

// Keep track of all members that are closed to allow the zip to be
// written properly.
func (self *MemberWriter) Close() error {
	err := self.WriteCloser.Close()
	self.writer_wg.Done()

	ContainerTracker.UpdateContainerWriter(self.owner.id, self.id,
		func(info *WriterInfo) {
			info.Closed = utils.GetTime().Now()
		})

	return err
}

type Container struct {
	config_obj *config_proto.Config

	id uint64

	// The underlying file writer
	fd io.WriteCloser

	// We use this name to track the container for debugging.
	name string

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

	stats_provider, _ := writer.(concurrent_zip.StatsWriter)

	res := &MemberWriter{
		WriteCloser:    writer,
		stats_provider: stats_provider,
		writer_wg:      &self.writer_wg,
		owner:          self,
		id:             utils.GetId(),
	}

	ContainerTracker.UpdateContainerWriter(self.id, res.id,
		func(info *WriterInfo) {
			info.Name = name
			info.Created = utils.GetTime().Now()
			if stats_provider != nil {
				stats := stats_provider.GetStats()
				info.TmpFile = stats.TmpFile
			}
		})

	return res, nil
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

	vql, err := vfilter.Parse(query.VQL)
	if err != nil {
		return 0, err
	}

	artifact_name := query.Name

	// Dont store un-named queries but run them anyway.
	if artifact_name == "" {
		query_log := actions.QueryLog.AddQuery(query.VQL)
		defer query_log.Close()

		for range vql.Eval(subctx, scope) {
			total_rows++
		}
		return total_rows, nil
	}

	// Record query progress for stats and debugging.
	query_log := actions.QueryLog.AddQuery(query.Name + ":" + query.VQL)
	defer query_log.Close()

	// The name to use in the zip file to store results from this artifact
	dest := ZipRootPath.Append(prefix.Components()...).Append(
		artifact_name + ".json")
	return self.WriteResultSet(subctx, config_obj, scope, format,
		dest.String(), vql.Eval(subctx, scope))
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

		files.Add(dest)
		defer func() {
			files.Remove(dest)
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

	files.Add(name)
	defer files.Remove(name)

	_, err = fd.Write(json.MustMarshalIndent(data))
	return err
}

func formatFilename(filename *accessors.OSPath, accessor string) string {
	result := filename.String()

	// These paths can be very large so we elide them.
	switch accessor {
	case "data":
		elided_result := "data:"
		for _, r := range result {
			if unicode.IsPrint(r) {
				elided_result += string(r)
			}
			if len(elided_result) > 50 {
				elided_result += " ..."
				break
			}
		}

		return elided_result

	case "sparse":
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
	mode os.FileMode,
	reader io.ReadSeeker) (res *uploads.UploadResponse, res_err error) {

	// The filename to store the file inside the zip - due to escaping
	// issues this may not be exactly the same as the file name we
	// receive.
	if store_as_name == nil {
		store_as_name = filename
	}

	result, closer := uploads.DeduplicateUploads(
		accessor, scope, store_as_name)
	defer closer(result)
	if result != nil {
		return result, nil
	}

	result = &uploads.UploadResponse{
		Path: formatFilename(filename, accessor),
		Size: uint64(expected_size),
	}

	// Avoid sending huge strings inside the JSON
	if accessor == "data" {
		result.Path = "data"
	} else if accessor == "" {
		accessor = "auto"
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

	// When uploading a directory we ensure that the name ends with a
	// / which will create a zip directory
	if mode.IsDir() && !strings.HasSuffix(result.StoredName, "/") {
		result.StoredName += "/"
	}

	scope.Log("Collecting file %s into %s (%v bytes)",
		formatFilename(filename, accessor), result.StoredName, expected_size)

	// Try to collect sparse files if possible
	err = self.maybeCollectSparseFile(ctx, scope, reader, result, mtime)
	if err == nil {
		self.mu.Lock()
		self.uploads = append(self.uploads, result)
		self.mu.Unlock()
		closer(result)
		return result, nil
	}

	writer, err := self.Create(result.StoredName, mtime)
	if err != nil {
		return nil, err
	}

	defer func() {
		res_err = writer.Close()
	}()

	files.Add(result.StoredName)
	defer files.Remove(result.StoredName)

	sha_sum := sha256.New()
	md5_sum := md5.New()

	// For very large files we need to emit some progress reporting.
	tee_writer, cancel := utils.NewDurationProgressWriter(
		ctx, func(byte_count int, duration time.Duration) {
			scope.Log("Wrote %v/%v into %v in %v\n",
				humanize.Bytes(uint64(byte_count)),
				humanize.Bytes(uint64(expected_size)),
				result.StoredName, duration)
		}, utils.NewTee(writer, sha_sum, md5_sum),
		time.Duration(10*time.Second))
	defer cancel()

	buff := pool.Get().(*[]byte)
	defer pool.Put(buff)

	count, err := utils.CopyWithBuffer(ctx, tee_writer, reader, *buff)
	if err != nil {
		result.StoredSize = uint64(count)
		result.Error = err.Error()
		closer(result)
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

	closer(result)
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

	files.Add(result.StoredName)
	defer files.Remove(result.StoredName)

	// For very large files we need to emit some progress reporting.
	tee_writer, cancel := utils.NewDurationProgressWriter(
		ctx, func(byte_count int, duration time.Duration) {
			scope.Log("Wrote %v into %v in %v\n",
				humanize.Bytes(uint64(byte_count)),
				result.StoredName, duration)
		}, writer, time.Duration(10*time.Second))
	defer cancel()

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

		run_writer := utils.NewTee(tee_writer, sha_sum, md5_sum)
		n, err := utils.CopyN(ctx, run_writer, range_reader, rng.Length)
		if err != nil {
			return err
		}

		// We were unable to fully copy this run - this could indicate
		// an issue with decompression of the ntfs for
		// example. However we still need to maintain alignment here
		// so we pad with zeros.
		if int64(n) < rng.Length {
			scope.Log("Unable to fully copy range %#v in %v - padding %v bytes",
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

		files.Add(idx_upload.StoredName)
		defer files.Remove(idx_upload.StoredName)

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

	// self.zip is the zip we actually write in, while
	// self.delegate_zip is the container zip. In the case where the
	// output is encrypted, self.zip is pointing at `data.zip` so it
	// must be closed **before** we close the containing zip (in
	// self.delegate_zip).
	self.zip.Close()
	files.Remove(self.name)

	// Only report the hash if we actually wrote something (few bytes
	// Make sure the delegate is closed **before** we close the
	// container zip.
	if self.delegate_zip != nil {
		files.Remove(self.name)
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

	ContainerTracker.UpdateContainer(self.id, func(info *ContainerInfo) {
		info.CloseTime = utils.GetTime().Now()
	})

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
	id := self.id
	self.stats_mu.Unlock()

	stats.TotalUploadedFiles = uint64(len(self.uploads))
	stats.TotalCompressedBytes = uint64(self.writer.Count())
	stats.TotalDuration = uint64(Clock.Now().Unix()) - stats.Timestamp
	stats.ActiveMembers = ContainerTracker.GetActiveMembers(id)

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
	files.Add(path)

	res, err := NewContainerFromWriter(path, config_obj,
		NewBufferedCloser(fd), password, level, metadata)
	if err != nil {
		return nil, err
	}

	ContainerTracker.UpdateContainer(res.id, func(info *ContainerInfo) {
		info.BackingFile = path
	})

	return res, nil
}

func NewContainerFromWriter(
	name string,
	config_obj *config_proto.Config, fd io.WriteCloser,
	password string, level int64, metadata []vfilter.Row) (*Container, error) {

	var err error

	if level < 0 || level > 9 {
		level = 5
	}

	fd = utils.NewBufferCloser(fd)

	sha_sum := sha256.New()

	result := &Container{
		id:         utils.GetId(),
		config_obj: config_obj,
		name:       name,
		fd:         fd,
		sha_sum:    sha_sum,
		writer:     utils.NewTee(fd, sha_sum),
		level:      int(level),
	}

	result.stats.Timestamp = uint64(Clock.Now().Unix())

	// We need to build a protected container.
	if password != "" {
		files.Add(name + "-delegate")
		result.delegate_zip = zip.NewWriter(result.writer)
		if metadata != nil && len(metadata) != 0 {
			fh, err := result.delegate_zip.Create("metadata.json")
			if err != nil {
				return nil, err
			}
			_, err = fh.Write(json.MustMarshalIndent(metadata))
			if err != nil {
				return nil, err
			}
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

		files.Add(name)
		result.zip = concurrent_zip.NewWriter(result.delegate_fd)
	} else {
		files.Add(name)
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
			defer fh.Close()

			_, err = fh.Write(json.MustMarshalIndent(metadata))
			if err != nil {
				return nil, err
			}
		}
	}

	ContainerTracker.UpdateContainer(result.id, func(info *ContainerInfo) {
		info.Name = result.name
		info.CreateTime = utils.GetTime().Now()
	})

	return result, nil
}
