package uploads

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/vfilter"
)

var (
	BUFF_SIZE = int64(1024 * 1024)
)

// An uploader delivering files from client to server.
type VelociraptorUploader struct {
	Responder *responder.Responder
	Count     int
}

func (self *VelociraptorUploader) Upload(
	ctx context.Context,
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor string,
	store_as_name string,
	expected_size int64,
	mtime time.Time,
	atime time.Time,
	ctime time.Time,
	btime time.Time,
	reader io.Reader) (
	*UploadResponse, error) {

	// Try to collect sparse files if possible
	result, err := self.maybeUploadSparse(
		ctx, scope, filename, accessor, store_as_name,
		expected_size, mtime, reader)
	if err == nil {
		return result, nil
	}

	if store_as_name == "" {
		store_as_name = filename.String()
	}

	result = &UploadResponse{
		Path:       filename.String(),
		StoredName: store_as_name,
	}

	offset := uint64(0)
	self.Count += 1

	md5_sum := md5.New()
	sha_sum := sha256.New()

	for {
		// Ensure there is a fresh allocation for every
		// iteration to prevent overwriting in flight buffers.
		buffer := make([]byte, BUFF_SIZE)
		read_bytes, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, err
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
				Path:     store_as_name,
				Accessor: accessor,
			},
			Offset:     offset,
			Size:       uint64(expected_size),
			StoredSize: uint64(expected_size),
			Mtime:      mtime.UnixNano(),
			Atime:      atime.UnixNano(),
			Ctime:      ctime.UnixNano(),
			Btime:      btime.UnixNano(),
			Data:       data,
		}

		select {
		case <-ctx.Done():
			return nil, errors.New("Cancelled!")

		default:
			// Send the packet to the server.
			self.Responder.AddResponse(ctx, &crypto_proto.VeloMessage{
				RequestId:  constants.TransferWellKnownFlowId,
				FileBuffer: packet})
		}

		offset += uint64(read_bytes)
		if err != nil && err != io.EOF {
			return nil, err
		}

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
	store_as_name string,
	ignored_expected_size int64,
	mtime time.Time,
	reader io.Reader) (
	*UploadResponse, error) {

	// Can the reader produce ranges?
	range_reader, ok := reader.(RangeReader)
	if !ok {
		return nil, errors.New("Not supported")
	}

	index := &actions_proto.Index{}

	// This is the response that will be passed into the VQL
	// engine.
	result := &UploadResponse{
		Path: filename.String(),
	}

	if store_as_name == "" {
		store_as_name = filename.String()
	}

	self.Count += 1

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

		self.Responder.AddResponse(ctx, &crypto_proto.VeloMessage{
			RequestId: constants.TransferWellKnownFlowId,
			FileBuffer: &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path:     store_as_name,
					Accessor: accessor,
				},
				Size:       uint64(real_size),
				StoredSize: 0,
				IsSparse:   is_sparse,
				Index:      index,
				Mtime:      mtime.UnixNano(),
				Eof:        true,
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
					Path:     store_as_name,
					Accessor: accessor,
				},
				Offset:     uint64(write_offset),
				Size:       uint64(real_size),
				StoredSize: uint64(expected_size),
				IsSparse:   is_sparse,
				Mtime:      mtime.UnixNano(),
				Data:       data,
			}

			select {
			case <-ctx.Done():
				return nil, errors.New("Cancelled!")

			default:
				// Send the packet to the server.
				self.Responder.AddResponse(ctx, &crypto_proto.VeloMessage{
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
	self.Responder.AddResponse(ctx, &crypto_proto.VeloMessage{
		RequestId: constants.TransferWellKnownFlowId,
		FileBuffer: &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				Path:     store_as_name,
				Accessor: accessor,
			},
			Size:       uint64(real_size),
			StoredSize: uint64(expected_size),
			IsSparse:   is_sparse,
			Offset:     uint64(write_offset),
			Index:      index,
			Eof:        true,
		},
	})

	result.Size = uint64(real_size)
	result.StoredSize = uint64(write_offset)
	result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
	result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))

	return result, nil
}
