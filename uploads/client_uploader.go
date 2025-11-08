package uploads

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	BUFF_SIZE  = int64(1024 * 1024)
	UPLOAD_CTX = "__uploads"
)

type Transaction struct {
	*actions_proto.UploadTransaction

	scope    vfilter.Scope
	sentinel bool
}

// An uploader delivering files from client to server.
type VelociraptorUploader struct {
	Responder responder.Responder

	mu           sync.Mutex
	ctx          context.Context
	cancel       func()
	transactions chan *Transaction
	logger       *log.Logger
	count        int
	id           uint64

	current map[int64]*actions_proto.UploadTransaction

	wg sync.WaitGroup
}

func NewVelociraptorUploader(
	ctx context.Context,
	logger *log.Logger,
	deadline time.Duration,
	responder responder.Responder) *VelociraptorUploader {
	sub_ctx, cancel := context.WithTimeout(ctx, deadline)

	res := &VelociraptorUploader{
		id:           utils.GetId(),
		Responder:    responder,
		ctx:          sub_ctx,
		cancel:       cancel,
		logger:       logger,
		current:      make(map[int64]*actions_proto.UploadTransaction),
		transactions: make(chan *Transaction, 10000),
	}

	gClientUploaderTracker.Register(res.id, res)

	res.wg.Add(1)
	go res.Start(ctx)
	return res
}

func (self *VelociraptorUploader) Abort() {
	self.cancel()
	self.flushOutstanding()
}

// Clear the queue from any transactions outstanding.
func (self *VelociraptorUploader) flushOutstanding() {
	count := 0
outer:
	for {
		select {
		case t, ok := <-self.transactions:
			if !ok {
				return
			}

			// Ignore the sentinel
			if t.sentinel {
				continue
			}

			self.mu.Lock()
			delete(self.current, t.UploadId)
			self.mu.Unlock()

			count++

		default:
			break outer
		}
	}

	if count > 0 && self.logger != nil {
		self.logger.Printf(
			"ERROR:Client Uploader:Aborting client uploader with %v transactions outstanding: %v",
			count, utils.Cause(self.ctx))
	}
}

func (self *VelociraptorUploader) Start(ctx context.Context) {
	defer self.wg.Done()
	defer self.flushOutstanding()

	for {
		select {
		case <-ctx.Done():
			return

		case t, ok := <-self.transactions:
			if !ok {
				return
			}

			// This represents last transaction - we can quit.
			if t.sentinel {
				return
			}

			resp, err := self.processTransaction(t)
			if err == utils.CancelledError {
				return
			}

			if err != nil {
				t.scope.Log("Client Uploader: %v", err)
				resp = &UploadResponse{
					Error:      err.Error(),
					Path:       t.StoreAsName,
					Size:       uint64(t.ExpectedSize),
					StoredSize: uint64(t.ExpectedSize),
					StoredName: t.StoreAsName,
					Components: t.Components,
					Accessor:   t.Accessor,
					ID:         t.UploadId,
				}
			}

			t.Response, _ = json.MarshalString(resp)

			self.Responder.AddResponse(&crypto_proto.VeloMessage{
				RequestId:         constants.TransferWellKnownFlowId,
				UploadTransaction: t.UploadTransaction,
			})
		}
	}

}

func (self *VelociraptorUploader) processTransaction(t *Transaction) (
	*UploadResponse, error) {

	defer func() {
		self.Responder.FlowContext().DecTransaction()

		self.mu.Lock()
		defer self.mu.Unlock()

		delete(self.current, t.UploadId)
	}()

	fs, err := accessors.GetAccessor(t.Accessor, t.scope)
	if err != nil {
		return nil, err
	}

	file_name, err := fs.ParsePath(t.Filename)
	if err != nil {
		return nil, err
	}
	store_as_name := file_name

	if t.StoreAsName != "" {
		store_as_name, err = fs.ParsePath(t.StoreAsName)
		if err != nil {
			return nil, err
		}
	}

	reader, err := fs.OpenWithOSPath(file_name)
	if err != nil {
		return nil, err
	}

	self.wg.Add(1)
	return self._Upload(self.ctx,
		t.scope, file_name, t.Accessor, store_as_name,
		t.ExpectedSize,
		time.Unix(0, t.Mtime),
		time.Unix(0, t.Atime),
		time.Unix(0, t.Ctime),
		time.Unix(0, t.Btime),
		os.FileMode(t.Mode),
		reader,
		t.StartOffset,
		t.UploadId)

}

func (self *VelociraptorUploader) IncCount() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.count++
}

func (self *VelociraptorUploader) GetCount() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.count
}

func (self *VelociraptorUploader) GetTransactionCount() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return len(self.current)
}

func (self *VelociraptorUploader) Close() {
	// Signal the worker the uploads are done.
	self.transactions <- &Transaction{
		sentinel: true,
	}

	// Wait until all the transactions are processed.
	self.wg.Wait()

	gClientUploaderTracker.Unregister(self.id)
}

func (self *VelociraptorUploader) Upload(
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
	reader io.ReadSeeker) (result *UploadResponse, err error) {

	if mode.IsDir() {
		return nil, fmt.Errorf("%w: Directories not supported",
			fs.ErrInvalid)
	}

	if accessor == "" {
		accessor = "auto"
	}

	if store_as_name == nil {
		store_as_name = filename
	}

	result, closer := DeduplicateUploads(accessor, scope, store_as_name)
	defer closer(result)
	if result != nil {
		return result, nil
	}

	upload_id := self.Responder.NextUploadId()

	// Send the start of the transaction
	transaction := &actions_proto.UploadTransaction{
		Filename:     filename.String(),
		Accessor:     accessor,
		StoreAsName:  store_as_name.String(),
		Components:   store_as_name.Components,
		ExpectedSize: expected_size,
		Mtime:        mtime.UnixNano(),
		Atime:        atime.UnixNano(),
		Ctime:        ctime.UnixNano(),
		Btime:        btime.UnixNano(),
		Mode:         int64(mode),
		UploadId:     upload_id,
	}

	self.Responder.AddResponse(&crypto_proto.VeloMessage{
		RequestId:         constants.TransferWellKnownFlowId,
		UploadTransaction: transaction})

	// Resumable uploads
	if self.shouldUploadAsync(scope, accessor) {
		// Schedule the transaction for execution.
		self.ReplayTransaction(ctx, scope, transaction)

		// When we upload asynchronously we return an upload id which
		// can be used to track the upload (or resume it) in future.
		result = &UploadResponse{
			StoredName: store_as_name.String(),
			Accessor:   accessor,
			Components: store_as_name.Components[:],
			ID:         upload_id,
		}
		closer(result)
		return result, nil

	}

	self.wg.Add(1)
	result, err = self._Upload(ctx, scope, filename, accessor,
		store_as_name, expected_size, mtime, atime, ctime, btime,
		mode, reader, 0, upload_id)
	closer(result)
	return result, err
}

func (self *VelociraptorUploader) ReplayTransaction(
	ctx context.Context,
	scope vfilter.Scope,
	t *actions_proto.UploadTransaction) {

	transaction := &Transaction{
		UploadTransaction: t,
		scope:             scope,
	}

	self.Responder.FlowContext().IncTransaction()
	self.mu.Lock()
	self.current[t.UploadId] = t
	self.mu.Unlock()

	select {
	case <-ctx.Done():
	case self.transactions <- transaction:
	}
}

func (self *VelociraptorUploader) shouldUploadAsync(
	scope vfilter.Scope,
	accessor string) bool {
	if !vql_subsystem.GetBoolFromRow(
		scope, scope, constants.UPLOAD_IS_RESUMABLE) {
		return false
	}

	// Only certain accessors are eligible for resuming. The
	// requirements are that it is possible to re-open the reader from
	// the filename and accessor alone. Ie. that the reader is context
	// free and hermetic.

	// For example, the "scope" accessor requires the scope to be
	// recreated, the "s3" accessor requires configuration via the
	// scope etc.
	switch accessor {
	case "ntfs", "file", "ext4", "auto":
		return true
	}
	return false
}

func (self *VelociraptorUploader) _Upload(
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
	reader io.ReadSeeker,
	start_offset int64,
	upload_id int64) (*UploadResponse, error) {

	defer self.wg.Done()

	// Try to collect sparse files if possible
	result, err := self.maybeUploadSparse(
		ctx, scope, filename, accessor, store_as_name,
		expected_size, mtime, upload_id, reader)
	if err == nil {
		return result, nil
	}

	result = &UploadResponse{
		StoredName: store_as_name.String(),
		Accessor:   accessor,
		Components: store_as_name.Components[:],
		ID:         upload_id,
	}
	if accessor != "data" {
		result.Path = filename.String()
	}

	offset := uint64(start_offset)
	self.IncCount()

	md5_sum := md5.New()
	sha_sum := sha256.New()

	_, err = reader.Seek(start_offset, os.SEEK_SET)
	if err != nil {
		scope.Log("Unable to seek %v to offset %v",
			result.StoredName, start_offset)
	}
	for {
		// Ensure there is a fresh allocation for every
		// iteration to prevent overwriting in flight buffers.
		buffer := make([]byte, BUFF_SIZE)
		read_bytes, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, err
		}

		InstrumentWithDelay()

		data := buffer[:read_bytes]
		_, err = sha_sum.Write(data)
		if err != nil {
			return nil, err
		}

		_, err = md5_sum.Write(data)
		if err != nil {
			return nil, err
		}

		packet := &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				Path:       store_as_name.String(),
				Components: store_as_name.Components,
				Accessor:   accessor,
			},
			Offset:     offset,
			Size:       uint64(expected_size),
			StoredSize: offset + uint64(len(data)),
			Mtime:      mtime.UnixNano(),
			Atime:      atime.UnixNano(),
			Ctime:      ctime.UnixNano(),
			Btime:      btime.UnixNano(),
			Data:       data,
			DataLength: uint64(len(data)),

			// The number of the upload within the flow.
			UploadNumber: upload_id,
			Eof:          read_bytes == 0,
		}

		select {
		case <-ctx.Done():
			return nil, utils.CancelledError

		default:
			// Send the packet to the server.
			self.Responder.AddResponse(&crypto_proto.VeloMessage{
				RequestId:  constants.TransferWellKnownFlowId,
				FileBuffer: packet})
		}

		offset += uint64(read_bytes)
		if err != nil && err != io.EOF {
			return nil, err
		}

		// On the last packet send back the hashes into the query.
		if read_bytes == 0 {
			result.Size = offset
			result.StoredSize = offset
			result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
			result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))
			return result, nil
		}
	}
}

func (self *VelociraptorUploader) maybeUploadSparse(
	ctx context.Context,
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor string,
	store_as_name *accessors.OSPath,
	ignored_expected_size int64,
	mtime time.Time,
	upload_id int64,
	reader io.Reader) (
	*UploadResponse, error) {

	// Can the reader produce ranges?
	range_reader, ok := reader.(RangeReader)
	if !ok {
		return nil, errors.New("Not supported")
	}

	index := &actions_proto.Index{}

	if store_as_name == nil {
		store_as_name = filename
	}

	// This is the response that will be passed into the VQL
	// engine.
	result := &UploadResponse{
		StoredName: store_as_name.String(),
		Components: store_as_name.Components,
		Accessor:   accessor,
	}
	if accessor != "data" {
		result.Path = filename.String()
	}

	self.IncCount()

	md5_sum := md5.New()
	sha_sum := sha256.New()

	// Does the index contain any sparse runs?
	is_sparse := false

	// Read from the sparse file with read_offset and write to the
	// output file at write_offset. All ranges are written back to
	// back skipping sparse ranges. The index file will allow
	// users to reconstruct the sparse file if needed.
	read_offset := int64(0)
	write_offset := int64(0)

	// Adjust the expected size properly to the sum of all
	// non-sparse ranges and build the index file.
	ranges := range_reader.Ranges()

	// Inspect the ranges and prepare an index.
	expected_size := int64(0)
	real_size := int64(0)
	for _, rng := range ranges {
		file_length := rng.Length
		if rng.IsSparse {
			file_length = 0
		}

		index.Ranges = append(index.Ranges,
			&actions_proto.Range{
				FileOffset:     expected_size,
				OriginalOffset: rng.Offset,
				FileLength:     file_length,
				Length:         rng.Length,
			})

		if !rng.IsSparse {
			expected_size += rng.Length
		} else {
			is_sparse = true
		}

		if real_size < rng.Offset+rng.Length {
			real_size = rng.Offset + rng.Length
		}
	}

	// No ranges - just send a placeholder.
	if expected_size == 0 {
		if !is_sparse {
			index = nil
		}

		self.Responder.AddResponse(&crypto_proto.VeloMessage{
			RequestId: constants.TransferWellKnownFlowId,
			FileBuffer: &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path:       store_as_name.String(),
					Components: store_as_name.Components,
					Accessor:   accessor,
				},
				Size:         uint64(real_size),
				StoredSize:   uint64(expected_size),
				IsSparse:     is_sparse,
				Index:        index,
				Mtime:        mtime.UnixNano(),
				Eof:          true,
				UploadNumber: upload_id,
			},
		})

		result.Size = uint64(real_size)
		result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
		result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))
		return result, nil
	}

	// Send each range separately
	for _, rng := range ranges {
		// Ignore sparse ranges
		if rng.IsSparse {
			continue
		}

		// Range is not sparse - send it one buffer at the time.
		to_read := rng.Length
		read_offset = rng.Offset
		_, err := range_reader.Seek(read_offset, io.SeekStart)
		if err != nil {
			return nil, err
		}

		for to_read > 0 {
			to_read_buf := to_read

			// Ensure there is a fresh allocation for every
			// iteration to prevent overwriting in-flight buffers.
			if to_read_buf > BUFF_SIZE {
				to_read_buf = BUFF_SIZE
			}

			buffer := make([]byte, to_read_buf)
			read_bytes, err := range_reader.Read(buffer)
			// Hard read error - give up.
			if err != nil && err != io.EOF {
				return nil, err
			}

			InstrumentWithDelay()

			// End of range - go to the next range
			if read_bytes == 0 || err == io.EOF {
				to_read = 0
				continue
			}

			data := buffer[:read_bytes]
			_, err = sha_sum.Write(data)
			if err != nil {
				return nil, err
			}

			_, err = md5_sum.Write(data)
			if err != nil {
				return nil, err
			}

			packet := &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path:       store_as_name.String(),
					Components: store_as_name.Components,
					Accessor:   accessor,
				},
				Offset:       uint64(write_offset),
				Size:         uint64(real_size),
				StoredSize:   uint64(expected_size),
				IsSparse:     is_sparse,
				Mtime:        mtime.UnixNano(),
				Data:         data,
				DataLength:   uint64(len(data)),
				UploadNumber: upload_id,
			}

			select {
			case <-ctx.Done():
				return nil, utils.CancelledError

			default:
				// Send the packet to the server.
				self.Responder.AddResponse(&crypto_proto.VeloMessage{
					RequestId:  constants.TransferWellKnownFlowId,
					FileBuffer: packet})
			}

			to_read -= int64(read_bytes)
			write_offset += int64(read_bytes)
			read_offset += int64(read_bytes)
		}
	}

	// We did a sparse file, upload the index as well.
	if !is_sparse {
		index = nil
	}

	// Send an EOF as the last packet with no data. If the file
	// was sparse, also include the index in this packet. NOTE:
	// There should be only one EOF packet.
	self.Responder.AddResponse(&crypto_proto.VeloMessage{
		RequestId: constants.TransferWellKnownFlowId,
		FileBuffer: &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				Path:       store_as_name.String(),
				Components: store_as_name.Components,
				Accessor:   accessor,
			},
			Size:         uint64(real_size),
			StoredSize:   uint64(write_offset),
			IsSparse:     is_sparse,
			Offset:       uint64(write_offset),
			Index:        index,
			Eof:          true,
			UploadNumber: upload_id,
		},
	})

	result.Size = uint64(real_size)
	result.StoredSize = uint64(write_offset)
	result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
	result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))

	return result, nil
}

// Allow uploading to be slowed down to simulate slow networks or
// disks. Allows testing upload timeout and transactions.
var InstrumentWithDelay = func() {}

func init() {
	delay_str, pres := os.LookupEnv("VELOCIRAPTOR_SLOW_FILESYSTEM")
	if pres {
		delay, err := strconv.Atoi(delay_str)
		if err == nil {
			InstrumentWithDelay = func() {
				time.Sleep(time.Duration(delay) * time.Millisecond)
			}
		}
	}
}
