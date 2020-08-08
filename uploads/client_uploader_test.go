package uploads

import (
	"bytes"
	"context"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/responder"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type TestRangeReader struct {
	*bytes.Reader
	ranges []Range
}

func (self *TestRangeReader) Ranges() []Range {
	return self.ranges
}

// Combine the output of all fragments into a strings
func CombineOutput(name string, responses []*crypto_proto.GrrMessage) string {
	result := []byte{}

	for _, item := range responses {
		if item.FileBuffer.Pathspec.Path == name {
			result = append(result, item.FileBuffer.Data...)
		}
	}

	return string(result)
}

func GetIndex(responses []*crypto_proto.GrrMessage) []*actions_proto.Range {
	return responses[len(responses)-1].FileBuffer.Index.Ranges
}

func TestClientUploaderSparse(t *testing.T) {
	resp := responder.TestResponder()
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
		"foo", "ntfs", "", 1000, range_reader)
	responses := responder.GetTestResponses(resp)

	// Expected size is the combined sum of all ranges with data
	// in them
	assert.Equal(t, responses[0].FileBuffer.Size, uint64(12))

	assert.Equal(t, CombineOutput("foo", responses), "Hello hello ")
	goldie.Assert(t, "ClientUploaderSparse", json.MustMarshalIndent(
		responses))
}

// Test what happens when the underlying reader is shorter than the
// ranges.
func TestClientUploaderSparseWithEOF(t *testing.T) {
	resp := responder.TestResponder()
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
		"foo", "ntfs", "", 1000, range_reader)
	responses := responder.GetTestResponses(resp)

	// Expected size is the combined sum of all ranges with data
	// in them
	assert.Equal(t, responses[0].FileBuffer.Size, uint64(12))
	assert.Equal(t, CombineOutput("foo", responses), "Hello hi")
}

// Trying to upload a completely sparse file with no data but real
// size.
func TestClientUploaderCompletelySparse(t *testing.T) {
	resp := responder.TestResponder()
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
		"foo", "ntfs", "", 1000, range_reader)
	responses := responder.GetTestResponses(resp)

	// Expected size is the combined sum of all ranges with data
	// in them.
	assert.Equal(t, responses[0].FileBuffer.Size, uint64(0))
}

func TestClientUploaderSparseMultiBuffer(t *testing.T) {
	resp := responder.TestResponder()
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
		"foo", "ntfs", "", 1000, range_reader)
	responses := responder.GetTestResponses(resp)
	assert.Equal(t, CombineOutput("foo", responses), "Hello hello ")
	goldie.Assert(t, "ClientUploaderSparseMultiBuffer",
		json.MustMarshalIndent(responses))
}

func TestClientUploaderNoIndexIfNotSparse(t *testing.T) {
	resp := responder.TestResponder()
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
		"foo", "ntfs", "", 1000, range_reader)
	responses := responder.GetTestResponses(resp)
	assert.Equal(t, CombineOutput("foo", responses), "Hello hello ")

	// No idx written when there are no sparse ranges.
	assert.Equal(t, CombineOutput("foo.idx", responses), "")
}
