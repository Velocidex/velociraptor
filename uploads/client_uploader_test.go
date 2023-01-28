package uploads

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
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
		if item.FileBuffer.Pathspec.Path == name {
			result = append(result, item.FileBuffer.Data...)
		}
	}

	return string(result)
}

func GetIndex(responses []*crypto_proto.VeloMessage) []*actions_proto.Range {
	return responses[len(responses)-1].FileBuffer.Index.Ranges
}

func TestClientUploaderSparse(t *testing.T) {
	resp := responder.TestResponderWithFlowId(nil, "TestClientUploaderSparse")
	uploader := &VelociraptorUploader{
		Responder: resp,
	}

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
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = resp.Drain.Messages()
		return len(responses) > 0
	})

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
	resp := responder.TestResponderWithFlowId(
		nil, "TestClientUploaderSparseWithEOF")
	uploader := &VelociraptorUploader{
		Responder: resp,
	}

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
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = resp.Drain.Messages()
		return len(responses) > 0
	})

	// Expected size is the combined sum of all ranges with data
	// in them
	assert.Equal(t, responses[0].FileBuffer.StoredSize, uint64(12))
	assert.Equal(t, responses[0].FileBuffer.Size, uint64(18))
	assert.Equal(t, CombineOutput("/foo", responses), "Hello hi")
}

func TestClientUploader(t *testing.T) {
	responder_obj := responder.TestResponderWithFlowId(
		nil, "TestClientUploader")
	uploader := &VelociraptorUploader{
		Responder: responder_obj,
	}

	BUFF_SIZE = 10

	tmpfile, err := ioutil.TempFile("", "tmp*")
	assert.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte("Hello world"))
	assert.NoError(t, err)

	name := tmpfile.Name()
	tmpfile.Close()

	fd, err := os.Open(name)
	assert.NoError(t, err)

	ctx := context.Background()
	scope := vql_subsystem.MakeScope()

	resp, err := uploader.Upload(
		ctx, scope, getOSPath(name),
		"file", nil, 1000,
		nilTime, nilTime, nilTime, nilTime, fd)
	assert.NoError(t, err)
	assert.Equal(t, resp.Path, name)
	assert.Equal(t, resp.Size, uint64(11))
	assert.Equal(t, resp.StoredSize, uint64(11))
	assert.Equal(t, resp.Error, "")
}

// Trying to upload a completely sparse file with no data but real
// size.
func TestClientUploaderCompletelySparse(t *testing.T) {
	resp := responder.TestResponderWithFlowId(
		nil, "TestClientUploaderCompletelySparse")
	uploader := &VelociraptorUploader{
		Responder: resp,
	}

	BUFF_SIZE = 10000

	reader := &TestRangeReader{
		Reader: bytes.NewReader([]byte("Hello world hi")), // len=11
		ranges: []Range{
			{Offset: 0, Length: 6, IsSparse: true},
		},
	}
	range_reader, ok := interface{}(reader).(RangeReader)
	assert.Equal(t, ok, true)
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()

	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = resp.Drain.Messages()
		return len(responses) > 0
	})

	// Expected size is the combined sum of all ranges with data
	// in them.
	assert.Equal(t, responses[0].FileBuffer.StoredSize, uint64(0))
	assert.Equal(t, responses[0].FileBuffer.Size, uint64(6))
}

func TestClientUploaderSparseMultiBuffer(t *testing.T) {
	resp := responder.TestResponderWithFlowId(
		nil, fmt.Sprintf("Test%d", utils.GetId()))
	uploader := &VelociraptorUploader{
		Responder: resp,
	}

	// 2 bytes per message
	BUFF_SIZE = 2
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
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()

	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = resp.Drain.Messages()
		return CombineOutput("/foo", responses) == "Hello hello "
	})

	assert.Equal(t, CombineOutput("/foo", responses), "Hello hello ")
	for _, response := range responses {
		response.ResponseId = 0
		response.SessionId = ""
	}

	goldie.Assert(t, "ClientUploaderSparseMultiBuffer",
		json.MustMarshalIndent(responses))
}

// Upload multiple copies of the same file.

// * Each copy should have 2 messages - one with the full data and one
//   with EOF message.
// * Each message should have an upload ID incrementing from 0 for all
//   packets in the same file.
func TestClientUploaderUploadId(t *testing.T) {
	cancel := utils.MockTime(&utils.MockClock{MockNow: time.Unix(10, 10)})
	defer cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp := responder.TestResponderWithFlowId(nil, fmt.Sprintf("Test22"))
	uploader := &VelociraptorUploader{
		Responder: resp,
	}

	data := "Hello world"

	// Upload the file multiple times
	for i := 0; i < 5; i++ {
		fd := bytes.NewReader([]byte(data))
		ospath := accessors.MustNewPathspecOSPath(fmt.Sprintf("file_%d", i))
		scope := vql_subsystem.MakeScope()
		_, err := uploader.Upload(ctx, scope,
			ospath, "data", nil, int64(len(data)),
			nilTime, nilTime, nilTime, nilTime, fd)
		assert.NoError(t, err)
	}
	// Collection Succeeded
	resp.Return(ctx)
	resp.Close()

	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = resp.Drain.Messages()
		return len(responses) > 0
	})

	golden := ordereddict.NewDict().
		Set("responses", responses)

	goldie.Assert(t, "TestClientUploaderUploadId",
		json.MustMarshalIndent(golden))
}

func TestClientUploaderNoIndexIfNotSparse(t *testing.T) {
	resp := responder.TestResponderWithFlowId(
		nil, "TestClientUploaderNoIndexIfNotSparse")
	uploader := &VelociraptorUploader{
		Responder: resp,
	}

	// 2 bytes per message
	BUFF_SIZE = 2
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
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	uploader.maybeUploadSparse(ctx, scope,
		filename, "ntfs", nil, 1000, nilTime,
		resp.NextUploadId(),
		range_reader)

	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = resp.Drain.Messages()
		return len(responses) > 0
	})
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
