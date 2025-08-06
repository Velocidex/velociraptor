package sparse

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sync"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/zip"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	vfilter "www.velocidex.com/golang/vfilter"
)

type RangedReaderPath struct {
	JsonlRanges string `json:"jsonl"`
	Index       string `json:"index"`
}

func parseIndexRanges(serialized []byte) (*actions_proto.Index, error) {
	arg := &RangedReaderPath{}
	err := json.Unmarshal(serialized, arg)
	if err != nil {
		return nil, err
	}

	index := &actions_proto.Index{}

	if arg.JsonlRanges != "" {
		reader := bufio.NewReader(bytes.NewReader([]byte(arg.JsonlRanges)))
		for {
			row_data, err := reader.ReadBytes('\n')
			if err != nil || len(row_data) == 0 {
				return index, nil
			}

			item := &actions_proto.Range{}
			err = json.Unmarshal(row_data, item)
			if err == nil {
				index.Ranges = append(index.Ranges, item)
			}
		}
	}

	if arg.Index != "" {
		result := &actions_proto.Index{}
		err = json.Unmarshal([]byte(arg.Index), result)
		if err == nil {
			index.Ranges = append(index.Ranges, result.Ranges...)
		}
	}

	return index, nil
}

type RangedReader struct {
	mu     sync.Mutex
	size   int64
	offset int64

	// A file handle to the underlying file.
	handle    accessors.ReadSeekCloser
	reader_at io.ReaderAt
}

func (self *RangedReader) Read(buf []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	n, err := self.reader_at.ReadAt(buf, self.offset)
	self.offset += int64(n)

	// Range is past the end of file
	return n, err
}

func (self *RangedReader) Seek(offset int64, whence int) (int64, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	switch whence {
	case 0:
		self.offset = offset
	case 1:
		self.offset += offset
	case 2:
		self.offset = self.size
	}

	return int64(self.offset), nil
}

func (self *RangedReader) Close() error {
	return self.handle.Close()
}

func (self *RangedReader) LStat() (accessors.FileInfo, error) {
	return &SparseFileInfo{size: self.size}, nil
}

func GetRangedReaderFile(full_path *accessors.OSPath, scope vfilter.Scope) (
	zip.ReaderStat, error) {
	if len(full_path.Components) == 0 {
		return nil, fmt.Errorf("Ranged accessor expects a JSON sparse definition.")
	}

	// The Path is a serialized ranges map.
	index, err := parseIndexRanges([]byte(full_path.Components[0]))
	if err != nil {
		scope.Log("Ranged accessor expects ranges as path, for example: '[{Offset:0, Length: 10},{Offset:10,length:20}]'")
		return nil, err
	}

	pathspec := full_path.PathSpec()

	accessor, err := accessors.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("%v: did you provide a PathSpec?", err)
		return nil, err
	}

	fd, err := accessor.Open(pathspec.GetDelegatePath())
	if err != nil {
		scope.Log("sparse: Failed to open delegate %v: %v",
			pathspec.GetDelegatePath(), err)
		return nil, err
	}

	// Devices can not be stat'ed
	size := int64(0)
	if len(index.Ranges) > 0 {
		last := index.Ranges[len(index.Ranges)-1]
		size = last.FileOffset + last.FileLength
	}

	return &RangedReader{
		handle: fd,
		size:   size,
		reader_at: &utils.RangedReader{
			ReaderAt: utils.MakeReaderAtter(fd),
			Index:    index,
		},
	}, nil
}

func init() {
	accessors.Register(accessors.DescribeAccessor(
		zip.NewGzipFileSystemAccessor(
			accessors.MustNewPathspecOSPath(""), GetRangedReaderFile),
		accessors.AccessorDescriptor{
			Name:        "ranged",
			Description: `Reconstruct sparse files from idx and base`,
		}))
}
