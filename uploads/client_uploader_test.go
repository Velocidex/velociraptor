package uploads

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var (
	nilTime  = time.Unix(0, 0)
	filename = accessors.MustNewLinuxOSPath("foo")
)

type TestRangeReader struct {
	*bytes.Reader
	ranges []Range
}

func (self *TestRangeReader) Ranges() []Range {
	return self.ranges
}

// Combine the output of all fragments into a strings
func CombineOutput(name string, responses []*crypto_proto.VeloMessage) string {
	result := []byte{}

	for _, item := range responses {
		if item.FileBuffer != nil &&
			item.FileBuffer.Pathspec.Path == name {
			result = append(result, item.FileBuffer.Data...)
		}
	}

	return string(result)
}

func GetIndex(responses []*crypto_proto.VeloMessage) []*actions_proto.Range {
	return responses[len(responses)-1].FileBuffer.Index.Ranges
}

func TestClientUploaderSparse(t *testing.T) {
	ctx := context.Background()

	resp := responder.TestResponderWithFlowId(nil, "TestClientUploaderSparse")
	uploader := NewVelociraptorUploader(ctx, nil, 0, resp)
	defer uploader.Close()

	BUFF_SIZE = 10000

	reader := &TestRangeReader{
		Reader: bytes.NewReader([]byte(
			"Hello world hello world")),
		ranges: []Range{
			{Offset: 0, Length: 6, IsSparse: false},
			{Offset: 6, Length: 6, IsSparse: true},
			{Offset: 12, Length: 6, IsSparse: false},
		},
	}
	range_reader, ok := interface{}(reader).(RangeReader)
	assert.Equal(t, ok, true)

	scope := vql_subsystem.MakeScope()
	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	responses := resp.Drain.WaitForEof(t)

	// Expected size is the combined sum of all ranges with data
	// in them
	assert.Equal(t, responses[0].FileBuffer.StoredSize, uint64(12))
	assert.Equal(t, responses[0].FileBuffer.Size, uint64(18))

	assert.Equal(t, CombineOutput("/foo", responses), "Hello hello ")
	for _, response := range responses {
		response.ResponseId = 0
	}
	goldie.Assert(t, "ClientUploaderSparse", json.MustMarshalIndent(responses))
}

// Test what happens when the underlying reader is shorter than the
// ranges.
func TestClientUploaderSparseWithEOF(t *testing.T) {
	ctx := context.Background()
	resp := responder.TestResponderWithFlowId(
		nil, "TestClientUploaderSparseWithEOF")
	uploader := NewVelociraptorUploader(ctx, nil, 0, resp)
	defer uploader.Close()

	BUFF_SIZE = 10000

	reader := &TestRangeReader{
		Reader: bytes.NewReader([]byte("Hello world hi")), // len=11
		ranges: []Range{
			{Offset: 0, Length: 6, IsSparse: false},
			{Offset: 6, Length: 6, IsSparse: true},

			// Range exceeds size of reader.
			{Offset: 12, Length: 6, IsSparse: false},
		},
	}
	range_reader, ok := interface{}(reader).(RangeReader)
	assert.Equal(t, ok, true)

	scope := vql_subsystem.MakeScope()
	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = resp.Drain.Messages()
		return CombineOutput("/foo", responses) == "Hello hi"
	})

	// Expected size is the combined sum of all ranges with data
	// in them
	assert.Equal(t, responses[0].FileBuffer.StoredSize, uint64(12))
	assert.Equal(t, responses[0].FileBuffer.Size, uint64(18))
	assert.Equal(t, CombineOutput("/foo", responses), "Hello hi")
}

func TestClientUploaderMultipleBuffers(t *testing.T) {
	ctx := context.Background()

	cancel := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer cancel()

	responder_obj := responder.TestResponderWithFlowId(
		nil, "TestClientUploader")
	uploader := NewVelociraptorUploader(ctx, nil, 0, responder_obj)
	defer uploader.Close()

	BUFF_SIZE = 10

	scope := vql_subsystem.MakeScope()

	resp, err := uploader.Upload(
		ctx, scope, accessors.MustNewLinuxOSPath("test.txt"),
		"file", nil,

		// Expected_size
		1000, nilTime, nilTime, nilTime, nilTime, 0,
		bytes.NewReader([]byte("Hello world Hello world")))
	assert.NoError(t, err)

	responder_obj.Close()

	var messages []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		messages = responder_obj.Drain.Messages()
		for _, r := range messages {
			if r.FlowStats != nil {
				return true
			}
		}
		return false
	})

	golden := ordereddict.NewDict().
		Set("VQLResponse", resp).
		Set("Messages", messages)
	goldie.Assert(t, "TestClientUploaderMultipleBuffers",
		json.MustMarshalIndent(golden))
}

func TestClientUploaderMultipleUploads(t *testing.T) {
	ctx := context.Background()

	cancel := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer cancel()

	responder_obj := responder.TestResponderWithFlowId(
		nil, "TestClientUploader")
	uploader := NewVelociraptorUploader(ctx, nil, 0, responder_obj)
	defer uploader.Close()

	BUFF_SIZE = 1000

	scope := vql_subsystem.MakeScope()

	var resp interface{}
	var err error

	for i := 0; i < 2; i++ {
		resp, err = uploader.Upload(
			ctx, scope, accessors.MustNewLinuxOSPath(
				fmt.Sprintf("test23%v.txt", i)),
			"file", nil,

			// Expected_size
			1000, nilTime, nilTime, nilTime, nilTime, 0,
			bytes.NewReader([]byte("Hello world")))
		assert.NoError(t, err)
	}

	responder_obj.Close()

	var messages []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		messages = responder_obj.Drain.Messages()
		for _, r := range messages {
			if r.FlowStats != nil {
				return true
			}
		}
		return false
	})

	golden := ordereddict.NewDict().
		Set("VQLResponse", resp).
		Set("Messages", messages)
	goldie.Assert(t, "TestClientUploaderMultipleUploads",
		json.MustMarshalIndent(golden))
}

// Trying to upload a completely sparse file with no data but real
// size.
func TestClientUploaderCompletelySparse(t *testing.T) {
	ctx := context.Background()

	resp := responder.TestResponderWithFlowId(
		nil, "TestClientUploaderCompletelySparse")
	uploader := NewVelociraptorUploader(ctx, nil, 0, resp)
	defer uploader.Close()

	BUFF_SIZE = 10000

	reader := &TestRangeReader{
		Reader: bytes.NewReader([]byte("Hello world hi")), // len=11
		ranges: []Range{
			{Offset: 0, Length: 6, IsSparse: true},
		},
	}
	range_reader, ok := interface{}(reader).(RangeReader)
	assert.Equal(t, ok, true)

	scope := vql_subsystem.MakeScope()

	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	responses := resp.Drain.WaitForEof(t)

	// Expected size is the combined sum of all ranges with data
	// in them.
	assert.Equal(t, responses[0].FileBuffer.StoredSize, uint64(0))
	assert.Equal(t, responses[0].FileBuffer.Size, uint64(6))
}

func TestClientUploaderSparseMultiBuffer(t *testing.T) {
	ctx := context.Background()

	cancel := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer cancel()

	resp := responder.TestResponderWithFlowId(
		nil, fmt.Sprintf("Test%d", utils.GetId()))
	uploader := NewVelociraptorUploader(ctx, nil, 0, resp)
	defer uploader.Close()

	// 2 bytes per message
	BUFF_SIZE = 2
	defer func() {
		BUFF_SIZE = 1000
	}()

	reader := &TestRangeReader{
		Reader: bytes.NewReader([]byte(
			"Hello world hello world")),
		ranges: []Range{
			{Offset: 0, Length: 6, IsSparse: false},
			{Offset: 6, Length: 6, IsSparse: true},
			{Offset: 12, Length: 6, IsSparse: false},
		},
	}
	range_reader, ok := interface{}(reader).(RangeReader)
	assert.Equal(t, ok, true)
	scope := vql_subsystem.MakeScope()

	upload_resp, err := uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)
	assert.NoError(t, err)

	resp.Close()

	// Wait for the status message
	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = resp.Drain.Messages()
		return len(responses) > 0 && responses[len(responses)-1].FlowStats != nil
	})

	assert.Equal(t, CombineOutput("/foo", responses), "Hello hello ")
	for _, response := range responses {
		response.ResponseId = 0
		response.SessionId = ""
	}

	golden := ordereddict.NewDict().
		Set("VQLResponse", upload_resp).
		Set("Messages", responses)

	goldie.Assert(t, "ClientUploaderSparseMultiBuffer",
		json.MustMarshalIndent(golden))
}

// Upload multiple files.

//   - Each file should have 2 messages - one with the full data and one
//     with EOF message.
//   - Each message should have an upload ID incrementing from 0 for all
//     packets in the same file.
func TestClientUploaderUploadId(t *testing.T) {
	cancel := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp := responder.TestResponderWithFlowId(nil, fmt.Sprintf("Test22"))
	defer resp.Close()

	uploader := NewVelociraptorUploader(ctx, nil, 0, resp)
	defer uploader.Close()

	data := "Hello world"
	scope := vql_subsystem.MakeScope()

	// Upload the file multiple times
	for i := 0; i < 5; i++ {
		fd := bytes.NewReader([]byte(data))
		ospath := accessors.MustNewLinuxOSPath(fmt.Sprintf("file_%d", i))
		_, err := uploader.Upload(ctx, scope,
			ospath, "data", nil, int64(len(data)),
			nilTime, nilTime, nilTime, nilTime, 0, fd)
		assert.NoError(t, err)
	}

	// 3 messages per file:
	//
	// 1. UploadTransaction,
	// 2. FileBuffer with data
	// 3. FileBuffer with EOF
	responses := resp.Drain.WaitForMessage(t, 3*5)
	golden := ordereddict.NewDict().Set("responses", responses)

	goldie.Assert(t, "TestClientUploaderUploadId",
		json.MustMarshalIndent(golden))
}

// Upload multiple copies of the same file.  The client should
// deduplicate the files based on store_as_name so only actually
// upload a single file.
func TestClientUploaderDeduplicateStoreAsName(t *testing.T) {
	cancel := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	resp := responder.TestResponderWithFlowId(nil, fmt.Sprintf("Test23"))
	defer resp.Close()

	uploader := NewVelociraptorUploader(ctx, nil, 0, resp)
	defer uploader.Close()

	data := "Hello world"

	// All uploads use the same output filename.
	store_as_name := accessors.MustNewLinuxOSPath("TestFile.txt")
	scope := vql_subsystem.MakeScope()

	// Upload the file multiple times
	for i := 0; i < 5; i++ {
		fd := bytes.NewReader([]byte(data))

		// Only deduplicate on store_as_name - input files may be
		// different each time.
		ospath := accessors.MustNewLinuxOSPath(fmt.Sprintf("file_%d", i))
		_, err := uploader.Upload(ctx, scope,
			ospath, "data", store_as_name, int64(len(data)),
			nilTime, nilTime, nilTime, nilTime, 0, fd)
		assert.NoError(t, err)
	}

	responses := resp.Drain.WaitForEof(t)
	var eof_responses []*crypto_proto.VeloMessage
	for _, i := range responses {
		if i.FileBuffer != nil && i.FileBuffer.Eof {
			eof_responses = append(eof_responses, i)
		}
	}

	// Only two responses corresponding to one actual upload.
	assert.Equal(t, 1, len(eof_responses))

	golden := ordereddict.NewDict().
		Set("responses", responses)

	goldie.Assert(t, "TestClientUploaderDeduplicateStoreAsName",
		json.MustMarshalIndent(golden))
}

func TestClientUploaderNoIndexIfNotSparse(t *testing.T) {
	ctx := context.Background()

	resp := responder.TestResponderWithFlowId(
		nil, "TestClientUploaderNoIndexIfNotSparse")
	uploader := NewVelociraptorUploader(ctx, nil, 0, resp)
	defer uploader.Close()

	// 2 bytes per message
	BUFF_SIZE = 2
	defer func() {
		BUFF_SIZE = 1000
	}()

	reader := &TestRangeReader{
		Reader: bytes.NewReader([]byte(
			"Hello world hello world")),
		ranges: []Range{
			{Offset: 0, Length: 6, IsSparse: false},
			{Offset: 12, Length: 6, IsSparse: false},
		},
	}
	range_reader, ok := interface{}(reader).(RangeReader)
	assert.Equal(t, ok, true)

	scope := vql_subsystem.MakeScope()
	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	responses := resp.Drain.WaitForEof(t)
	assert.Equal(t, CombineOutput("/foo", responses), "Hello hello ")

	// No idx written when there are no sparse ranges.
	assert.Equal(t, CombineOutput("/foo.idx", responses), "")
}

func getOSPath(filename string) *accessors.OSPath {
	if runtime.GOOS == "windows" {
		return accessors.MustNewWindowsOSPath(filename)
	} else {
		return accessors.MustNewLinuxOSPath(filename)
	}
}
