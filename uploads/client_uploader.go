package uploads

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

const (
	BUFF_SIZE = 1024 * 1024
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

	// Try to collect sparse files if possible
	result, err := self.maybeUploadSparse(
		ctx, scope, filename, accessor, store_as_name,
		expected_size, reader)
	if err == nil {
		return result, nil
	}

	result = &api.UploadResponse{
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
		buffer := make([]byte, BUFF_SIZE)
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

func (self *VelociraptorUploader) maybeUploadSparse(
	ctx context.Context,
	scope *vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	expected_size int64,
	reader io.Reader) (
	*api.UploadResponse, error) {

	// Can the reader produce ranges?
	range_reader, ok := reader.(RangeReader)
	if !ok {
		return nil, errors.New("Not supported")
	}

	result := &api.UploadResponse{
		Path: filename,
	}

	if store_as_name == "" {
		store_as_name = filename
	}

	self.Count += 1

	md5_sum := md5.New()
	sha_sum := sha256.New()

	index := []*ordereddict.Dict{}

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
	if len(ranges) == 0 {
		// No ranges - just send a placeholder.
		self.Responder.AddResponse(&crypto_proto.GrrMessage{
			RequestId: constants.TransferWellKnownFlowId,
			FileBuffer: &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path:     store_as_name,
					Accessor: accessor,
				},
			},
		})
		return result, nil
	}

	// Inspect the ranges and prepare an index.
	expected_size = 0
	for _, rng := range ranges {
		file_length := rng.Length
		if rng.IsSparse {
			file_length = 0
		}

		index = append(index, ordereddict.NewDict().
			Set("file_offset", expected_size).
			Set("original_offset", rng.Offset).
			Set("file_length", file_length).
			Set("length", rng.Length))

		if !rng.IsSparse {
			expected_size += rng.Length
		} else {
			is_sparse = true
		}
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
		range_reader.Seek(read_offset, os.SEEK_SET)

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
			if read_bytes == 0 {
				continue
			}

			data := buffer[:read_bytes]
			sha_sum.Write(data)
			md5_sum.Write(data)

			packet := &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path:     store_as_name,
					Accessor: accessor,
				},
				Offset: uint64(write_offset),
				Size:   uint64(expected_size),
				Data:   data,
				Eof:    write_offset+int64(read_bytes) >= expected_size,
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

			to_read -= int64(read_bytes)
			write_offset += int64(read_bytes)
			read_offset += int64(read_bytes)
		}
	}

	// We did a sparse file, upload the index as well.
	if is_sparse {
		serialized, err := utils.DictsToJson(index, nil)
		if err != nil {
			return nil, err
		}
		self.Responder.AddResponse(&crypto_proto.GrrMessage{
			RequestId: constants.TransferWellKnownFlowId,
			FileBuffer: &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path:     store_as_name + ".idx",
					Accessor: accessor,
				},
				Offset: 0,
				Size:   uint64(len(serialized)),
				Data:   serialized,
				Eof:    true,
			},
		})
	}

	result.Size = uint64(write_offset)
	result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
	result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))

	return result, nil
}
