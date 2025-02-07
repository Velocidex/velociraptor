package http_comms

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
)

var (
	hook *test.Hook
)

func getTempFile(t *testing.T) string {
	fd, err := tempfile.TempFile("")
	assert.NoError(t, err)
	defer os.Remove(fd.Name())
	defer fd.Close()

	return fd.Name()
}

func createRB(t *testing.T, filename string) (*FileBasedRingBuffer, *responder.FlowManager) {
	config_obj := config.GetDefaultConfig()
	config_obj.Client.LocalBuffer.FilenameLinux = filename
	config_obj.Client.LocalBuffer.FilenameWindows = filename
	config_obj.Client.LocalBuffer.FilenameDarwin = filename

	null_logger, new_hook := test.NewNullLogger()
	logger := &logging.LogContext{Logger: null_logger}
	hook = new_hook

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flow_manager := responder.NewFlowManager(ctx, config_obj, "")

	local_buffer_name := getLocalBufferName(config_obj)
	ring_buffer, err := NewFileBasedRingBuffer(ctx, config_obj,
		local_buffer_name, flow_manager, logger)
	assert.NoError(t, err)

	return ring_buffer, flow_manager
}

func openRB(t *testing.T, filename string,
	flow_manager *responder.FlowManager) *FileBasedRingBuffer {
	config_obj := config.GetDefaultConfig()
	config_obj.Client.LocalBuffer.FilenameLinux = filename
	config_obj.Client.LocalBuffer.FilenameWindows = filename
	config_obj.Client.LocalBuffer.FilenameDarwin = filename

	null_logger, new_hook := test.NewNullLogger()
	logger := &logging.LogContext{Logger: null_logger}
	hook = new_hook

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ring_buffer, err := OpenFileBasedRingBuffer(ctx, config_obj, flow_manager, logger)
	assert.NoError(t, err)

	return ring_buffer
}

func TestRingBuffer(t *testing.T) {
	PREPARE_FOR_TESTS = true

	filename := getTempFile(t)
	test_string := "Hello"    // 5 bytes
	test_string2 := "Goodbye" // 7 bytes

	defer os.Remove(filename)

	ring_buffer, flow_manager := createRB(t, filename)
	ring_buffer.Enqueue([]byte(test_string))

	st, err := os.Stat(filename)
	assert.NoError(t, err)

	// Check that there is a single enqued buffer.
	assert.Equal(t,
		FirstRecordOffset+
			8+ // Length of item
			int64(len(test_string)),
		st.Size())

	// Open and enqueue another message
	ring_buffer = openRB(t, filename, flow_manager)

	// First message available.
	assert.Equal(t, ring_buffer.header.AvailableBytes,
		int64(len(test_string)))

	// Enqueue another message.
	ring_buffer.Enqueue([]byte(test_string2))

	// The file contains two messages.
	st, err = os.Stat(filename)
	assert.NoError(t, err)
	assert.Equal(t,
		FirstRecordOffset+
			8+ // Length of item
			int64(len(test_string))+
			8+
			int64(len(test_string2)),
		st.Size())

	// Lease one message from the buffer.
	ring_buffer = openRB(t, filename, flow_manager)

	// Two messages available.
	assert.Equal(t, ring_buffer.header.AvailableBytes,
		int64(len(test_string))+int64(len(test_string2)))

	// Lease a message
	lease := ring_buffer.Lease(1)

	assert.Equal(t, lease, []byte(test_string))

	// Second message available still.
	assert.Equal(t, ring_buffer.header.AvailableBytes,
		int64(len(test_string2)))

	// First message leased.
	assert.Equal(t, ring_buffer.header.LeasedBytes,
		int64(len(test_string)))

	// Since we did not commit the last message - opening again
	// will replay that same one.
	ring_buffer = openRB(t, filename, flow_manager)

	// Two messages available.
	assert.Equal(t, ring_buffer.header.AvailableBytes,
		int64(len(test_string))+int64(len(test_string2)))

	// Lease a message
	lease = ring_buffer.Lease(1)
	assert.Equal(t, lease, []byte(test_string))

	// Commit the message this time and close the file.
	ring_buffer.Commit()

	ring_buffer = openRB(t, filename, flow_manager)

	// Now only the second message is available.
	assert.Equal(t, ring_buffer.header.AvailableBytes,
		int64(len(test_string2)))

	// But the file contains both messages still.
	st, err = os.Stat(filename)
	assert.NoError(t, err)
	assert.Equal(t,
		FirstRecordOffset+
			8+ // Length of item
			int64(len(test_string))+
			8+
			int64(len(test_string2)),
		st.Size())

	ring_buffer = openRB(t, filename, flow_manager)

	// Leasing the second message now
	lease = ring_buffer.Lease(1)
	assert.Equal(t, lease, []byte(test_string2))

	// No messages are available now.
	assert.Equal(t, ring_buffer.header.AvailableBytes, int64(0))

	// But second message is currently leased - if we crash it
	// will be replayed.
	assert.Equal(t, ring_buffer.header.LeasedBytes,
		int64(len(test_string2)))

	// But the file contains both messages still.
	st, err = os.Stat(filename)
	assert.NoError(t, err)
	assert.Equal(t,
		FirstRecordOffset+
			8+ // Length of item
			int64(len(test_string))+
			8+
			int64(len(test_string2)),
		st.Size())

	// Now commit the lease.
	ring_buffer.Commit()

	// This should now truncate the file since there are no more
	// AvailableBytes and we committed the last outstanding
	// message.
	assert.Equal(t, ring_buffer.header.AvailableBytes, int64(0))
	assert.Equal(t, ring_buffer.header.LeasedBytes, int64(0))

	st, err = os.Stat(filename)
	assert.NoError(t, err)
	assert.Equal(t, int64(FirstRecordOffset), st.Size())
}

// Test that corrupted ring buffers are reset to a sane state. We
// inject errors into the file and check that we are hitting the right
// conditions based on the logged messages. After each error the file
// should be reset to its original virgin state.
func TestRingBufferCorruption(t *testing.T) {
	filename := getTempFile(t)
	test_string := "Hello"

	defer os.Remove(filename)

	ring_buffer, flow_manager := createRB(t, filename)
	ring_buffer.Enqueue([]byte(test_string))

	// Corrupt the file.
	fd, err := os.OpenFile(filename, os.O_RDWR, 0700)
	assert.NoError(t, err)

	fd.Seek(FirstRecordOffset, os.SEEK_SET)
	n, err := fd.Write([]byte{20, 0, 0, 0, 0, 0, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, n, 8)
	fd.Close()

	ring_buffer = openRB(t, filename, flow_manager)

	// Possible corruption detected - expected item of length 20 received 5.
	lease := ring_buffer.Lease(1)
	assert.Nil(t, lease)

	assert.Equal(t, checkLogMessage(hook,
		"Possible corruption detected - expected item of length 20 received 5."), true)

	st, err := os.Stat(filename)
	assert.NoError(t, err)
	assert.Equal(t, int64(FirstRecordOffset), st.Size())

	// Create a very short file.
	os.Remove(filename)

	fd, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
	assert.NoError(t, err)
	n, err = fd.Write([]byte{20, 0, 0, 0, 0, 0, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, n, 8)
	fd.Close()

	ring_buffer = openRB(t, filename, flow_manager)

	assert.Equal(t, true, checkLogMessage(hook,
		"Possible corruption detected: file too short."))

	assert.Equal(t, int64(FirstRecordOffset), ring_buffer.header.WritePointer)

	// Invalid header.
	fd, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
	assert.NoError(t, err)
	fd.Seek(0, 0)
	n, err = fd.Write([]byte{20, 0, 0, 0, 0, 0, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, n, 8)
	fd.Close()

	ring_buffer = openRB(t, filename, flow_manager)

	assert.Equal(t, checkLogMessage(hook,
		"Possible corruption detected: Invalid header length."), true)

	assert.Equal(t, int64(FirstRecordOffset), ring_buffer.header.WritePointer)
	ring_buffer.Enqueue([]byte(test_string))

	// Create a very large items length.
	fd, err = os.OpenFile(filename, os.O_RDWR, 0700)
	assert.NoError(t, err)
	fd.Seek(FirstRecordOffset, os.SEEK_SET)
	n, err = fd.Write([]byte{20, 0, 0, 0xff, 0xff, 0, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, n, 8)
	fd.Close()

	ring_buffer = openRB(t, filename, flow_manager)

	// Leasing the second message now
	lease = ring_buffer.Lease(1)
	assert.Equal(t, len(lease), 0)

	assert.Equal(t, checkLogMessage(hook,
		"Possible corruption detected - item length is too large."), true)

	assert.Equal(t, int64(FirstRecordOffset), ring_buffer.header.WritePointer)
}

func checkLogMessage(hook *test.Hook, msg string) bool {
	defer hook.Reset()

	for _, entry := range hook.AllEntries() {
		if entry.Message == msg {
			return true
		}
	}

	return false
}

func TestRingBufferCancellation(t *testing.T) {
	filename := getTempFile(t)
	defer os.Remove(filename)

	// Make SessionId unique for each test run
	message_list := &crypto_proto.MessageList{
		// Add some messages. We filter out large messages for
		// cancelled flows to preserve bandwidth to the server, but
		// FlowStats messgaes should still be allowed.
		Job: []*crypto_proto.VeloMessage{
			{
				SessionId: "F.1234" + filename,
				FileBuffer: &actions_proto.FileBuffer{
					Data: []byte("FileBuffer"),
				},
			},
			{
				SessionId: "F.1234" + filename,
				VQLResponse: &actions_proto.VQLResponse{
					JSONLResponse: "VQLResponse",
				},
			},
			{
				SessionId: "F.1234" + filename,
				FlowStats: &crypto_proto.FlowStats{
					QueryStatus: []*crypto_proto.VeloStatus{{
						Status:            crypto_proto.VeloStatus_GENERIC_ERROR,
						NamesWithResponse: []string{"FlowStats"},
					}},
				},
			},
		},
	}

	serialized_message_list, err := proto.Marshal(message_list)
	assert.NoError(t, err)

	// Queue the message
	ring_buffer, flow_manager := createRB(t, filename)
	ring_buffer.Enqueue([]byte(serialized_message_list))

	// Try to lease the message.
	ring_buffer = openRB(t, filename, flow_manager)
	lease := ring_buffer.Lease(1)
	assert.NotNil(t, lease)
	ring_buffer.Commit()

	// Make sure all messages are delivered
	assert.Equal(t, serialized_message_list, lease)

	// Queue the message
	ring_buffer.Enqueue([]byte(serialized_message_list))

	// Now cancel this flow ID.
	ctx := context.Background()
	flow_manager.Cancel(ctx, message_list.Job[0].SessionId)

	// Try to lease the message.
	ring_buffer = openRB(t, filename, flow_manager)
	lease = ring_buffer.Lease(10)
	assert.NotNil(t, lease)
	ring_buffer.Commit()

	// Should deliver only the FlowStats messages
	assert.Contains(t, string(lease), "FlowStats")
	assert.NotContains(t, string(lease), "FileBuffer")
	assert.NotContains(t, string(lease), "VQLResponse")

	// Queue more messages for a different flow
	for _, m := range message_list.Job {
		m.SessionId += "X"
	}
	serialized_message_list, err = proto.Marshal(message_list)
	assert.NoError(t, err)

	// Queue more messages
	ring_buffer.Enqueue([]byte(serialized_message_list))

	// Read the new messages back out
	lease = ring_buffer.Lease(10)
	assert.NotNil(t, lease)
	ring_buffer.Commit()

	// Make sure all messages are delivered
	assert.Equal(t, serialized_message_list, lease)
}
