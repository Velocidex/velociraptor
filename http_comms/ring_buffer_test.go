package http_comms

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	hook *test.Hook
)

func getTempFile(t *testing.T) string {
	fd, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	defer os.Remove(fd.Name())
	defer fd.Close()

	return fd.Name()
}

func createRB(t *testing.T, filename string) *FileBasedRingBuffer {
	config_obj := config.GetDefaultConfig()
	config_obj.Client.LocalBuffer.FilenameLinux = filename
	config_obj.Client.LocalBuffer.FilenameWindows = filename
	config_obj.Client.LocalBuffer.FilenameDarwin = filename

	null_logger, new_hook := test.NewNullLogger()
	logger := &logging.LogContext{null_logger}
	hook = new_hook

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ring_buffer, err := NewFileBasedRingBuffer(ctx, config_obj, logger)
	assert.NoError(t, err)

	return ring_buffer
}

func openRB(t *testing.T, filename string) *FileBasedRingBuffer {
	config_obj := config.GetDefaultConfig()
	config_obj.Client.LocalBuffer.FilenameLinux = filename
	config_obj.Client.LocalBuffer.FilenameWindows = filename
	config_obj.Client.LocalBuffer.FilenameDarwin = filename

	null_logger, new_hook := test.NewNullLogger()
	logger := &logging.LogContext{null_logger}
	hook = new_hook

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ring_buffer, err := OpenFileBasedRingBuffer(ctx, config_obj, logger)
	assert.NoError(t, err)

	return ring_buffer
}

func TestRingBuffer(t *testing.T) {
	filename := getTempFile(t)
	test_string := "Hello"    // 5 bytes
	test_string2 := "Goodbye" // 7 bytes

	defer os.Remove(filename)

	ring_buffer := createRB(t, filename)
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
	ring_buffer = openRB(t, filename)

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
	ring_buffer = openRB(t, filename)

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
	ring_buffer = openRB(t, filename)

	// Two messages available.
	assert.Equal(t, ring_buffer.header.AvailableBytes,
		int64(len(test_string))+int64(len(test_string2)))

	// Lease a message
	lease = ring_buffer.Lease(1)
	assert.Equal(t, lease, []byte(test_string))

	// Commit the message this time and close the file.
	ring_buffer.Commit()

	ring_buffer = openRB(t, filename)

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

	ring_buffer = openRB(t, filename)

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

	ring_buffer := createRB(t, filename)
	ring_buffer.Enqueue([]byte(test_string))

	// Corrupt the file.
	fd, err := os.OpenFile(filename, os.O_RDWR, 0700)
	assert.NoError(t, err)

	fd.Seek(FirstRecordOffset, os.SEEK_SET)
	n, err := fd.Write([]byte{20, 0, 0, 0, 0, 0, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, n, 8)
	fd.Close()

	ring_buffer = openRB(t, filename)

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

	ring_buffer = openRB(t, filename)

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

	ring_buffer = openRB(t, filename)

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

	ring_buffer = openRB(t, filename)

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
		Job: []*crypto_proto.VeloMessage{{
			SessionId: "F.1234" + filename,
		}},
	}

	serialized_message_list, err := proto.Marshal(message_list)
	assert.NoError(t, err)

	// Queue the message
	ring_buffer := createRB(t, filename)
	ring_buffer.Enqueue([]byte(serialized_message_list))

	// Try to lease the message.
	ring_buffer = openRB(t, filename)
	lease := ring_buffer.Lease(1)
	assert.NotNil(t, lease)

	assert.Equal(t, serialized_message_list, lease)

	// Queue the message
	ring_buffer.Enqueue([]byte(serialized_message_list))

	// Now cancel this flow ID.
	executor.Canceller.Cancel(message_list.Job[0].SessionId)

	// Try to lease the message.
	ring_buffer = openRB(t, filename)
	lease = ring_buffer.Lease(1)
	assert.NotNil(t, lease)

	// Should not lease the message any more since it is cancelled.
	assert.Equal(t, lease, []byte{})
}
