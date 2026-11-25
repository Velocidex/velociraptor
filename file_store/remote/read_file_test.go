package remote

import (
	"context"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	mock_proto "www.velocidex.com/golang/velociraptor/api/mock"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
)

func newTestFileReader(t *testing.T, data []byte, maxRecvSize uint64) (*FileReader, *mock_proto.MockAPIClient) {
	t.Helper()

	ctrl := gomock.NewController(t)
	client := mock_proto.NewMockAPIClient(ctrl)

	configObj := config.GetDefaultConfig()
	configObj.ApiConfig.MaxGrpcRecvSize = maxRecvSize

	client.EXPECT().VFSGetBuffer(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, request *api_proto.VFSFileBuffer, _ ...grpc.CallOption) (*api_proto.VFSFileBuffer, error) {
			start := int(request.Offset)
			end := start + int(request.Length)
			if start >= len(data) {
				return &api_proto.VFSFileBuffer{}, nil
			}
			if end > len(data) {
				end = len(data)
			}
			return &api_proto.VFSFileBuffer{Data: append([]byte(nil), data[start:end]...)}, nil
		}).AnyTimes()

	reader := &FileReader{
		filename:   path_specs.NewSafeFilestorePath("test", "remote-read").SetType(api.PATH_TYPE_FILESTORE_ANY),
		ctx:        context.Background(),
		config_obj: configObj,
		factory: func() (api_proto.APIClient, func() error, error) {
			return client, func() error { return nil }, nil
		},
	}

	return reader, client
}

func TestFileReaderReadPartialAndEOF(t *testing.T) {
	reader, _ := newTestFileReader(t, []byte("hello"), 0)

	buf := make([]byte, 3)
	n, err := reader.Read(buf)
	if err != nil || n != 3 || string(buf) != "hel" {
		t.Fatalf("first Read() = (%d, %q, %v), want (3, %q, nil)", n, buf, err, "hel")
	}

	buf = make([]byte, 4)
	n, err = reader.Read(buf)
	if err != io.EOF || n != 2 || string(buf[:n]) != "lo" {
		t.Fatalf("second Read() = (%d, %q, %v), want (2, %q, io.EOF)", n, buf[:n], err, "lo")
	}

	n, err = reader.Read(buf)
	if err != io.EOF || n != 0 {
		t.Fatalf("Read() at EOF = (%d, %v), want (0, io.EOF)", n, err)
	}
}

func TestFileReaderReadAcrossChunks(t *testing.T) {
	const maxRecvSize = uint64(24 * 1024)
	chunkSize := int(maxRecvSize) - 10240
	data := make([]byte, 2*chunkSize+137)
	for i := range data {
		data[i] = byte(i % 251)
	}

	reader, _ := newTestFileReader(t, data, maxRecvSize)
	buf := make([]byte, len(data))
	n, err := reader.Read(buf)
	if err != nil || n != len(data) {
		t.Fatalf("Read() = (%d, %v), want (%d, nil)", n, err, len(data))
	}
	for i := range data {
		if buf[i] != data[i] {
			t.Fatalf("Read() data differs at byte %d: got %d, want %d", i, buf[i], data[i])
		}
	}
	if reader.offset != int64(len(data)) {
		t.Fatalf("reader offset = %d, want %d", reader.offset, len(data))
	}
}

func TestFileReaderSeek(t *testing.T) {
	reader, _ := newTestFileReader(t, []byte("0123456789"), 0)
	buf := make([]byte, 4)

	n, err := reader.Read(buf)
	if err != nil || n != 4 || string(buf) != "0123" {
		t.Fatalf("initial Read() = (%d, %q, %v), want (4, %q, nil)", n, buf, err, "0123")
	}

	offset, err := reader.Seek(2, io.SeekStart)
	if err != nil || offset != 2 {
		t.Fatalf("Seek(2, io.SeekStart) = (%d, %v), want (2, nil)", offset, err)
	}
	n, err = reader.Read(buf)
	if err != nil || n != 4 || string(buf) != "2345" {
		t.Fatalf("Read() after SeekStart = (%d, %q, %v), want (4, %q, nil)", n, buf, err, "2345")
	}

	offset, err = reader.Seek(-2, io.SeekCurrent)
	if err != nil || offset != 4 {
		t.Fatalf("Seek(-2, io.SeekCurrent) = (%d, %v), want (4, nil)", offset, err)
	}
	n, err = reader.Read(buf)
	if err != nil || n != 4 || string(buf) != "4567" {
		t.Fatalf("Read() after SeekCurrent = (%d, %q, %v), want (4, %q, nil)", n, buf, err, "4567")
	}

	offset, err = reader.Seek(-3, io.SeekEnd)
	if err != nil || offset != 7 {
		t.Fatalf("Seek(-3, io.SeekEnd) = (%d, %v), want (7, nil)", offset, err)
	}
	n, err = reader.Read(buf)
	if err != io.EOF || n != 3 || string(buf[:n]) != "789" {
		t.Fatalf("Read() after SeekEnd = (%d, %q, %v), want (3, %q, io.EOF)", n, buf[:n], err, "789")
	}
}

func TestFileReaderUsesConfiguredAndDefaultChunkSizes(t *testing.T) {
	tests := []struct {
		name        string
		maxRecvSize uint64
		chunkSize   int
	}{
		{
			name:      "default receive size",
			chunkSize: constants.GRPC_DEFAULT_TRANSFER_SIZE - 10240,
		},
		{
			name:        "configured receive size",
				maxRecvSize: 24 * 1024,
			chunkSize:   24*1024 - 10240,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.chunkSize+1)
			for i := range data {
				data[i] = byte(i % 251)
			}

			reader, client := newTestFileReader(t, data, tc.maxRecvSize)
			var mu sync.Mutex
			var requestLengths []uint32
			client.EXPECT().VFSGetBuffer(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, request *api_proto.VFSFileBuffer, _ ...grpc.CallOption) (*api_proto.VFSFileBuffer, error) {
					mu.Lock()
					requestLengths = append(requestLengths, request.Length)
					mu.Unlock()

					start := int(request.Offset)
					end := start + int(request.Length)
					if end > len(data) {
						end = len(data)
					}
					return &api_proto.VFSFileBuffer{Data: append([]byte(nil), data[start:end]...)}, nil
				}).Times(2)

			buf := make([]byte, len(data))
			n, err := reader.Read(buf)
			if err != nil || n != len(data) {
				t.Fatalf("Read() = (%d, %v), want (%d, nil)", n, err, len(data))
			}
			if len(requestLengths) != 2 {
				t.Fatalf("made %d requests, want 2", len(requestLengths))
			}
			for _, length := range requestLengths {
				if int(length) > tc.chunkSize {
					t.Fatalf("request length %d exceeds chunk size %d", length, tc.chunkSize)
				}
			}
			for i := range data {
				if buf[i] != data[i] {
					t.Fatalf("Read() data differs at byte %d: got %d, want %d", i, buf[i], data[i])
				}
			}
		})
	}
}
