package filesystem

import (
	"encoding/json"
	"io"
	"os"
	"sync"

	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

const PAGE_SIZE = 0x1000

func parseRanges(serialized []byte) ([]*uploads.Range, error) {
	ranges := []*uploads.Range{}
	err := json.Unmarshal(serialized, &ranges)
	if err != nil {
		return nil, err
	}

	result := make([]*uploads.Range, 0, len(ranges))
	offset := int64(0)
	for _, r := range ranges {
		if r.Offset != offset {
			result = append(result, &uploads.Range{
				Offset:   offset,
				Length:   r.Offset - offset,
				IsSparse: true,
			})
		}
		result = append(result, r)
		offset = r.Offset + r.Length
	}

	return result, nil
}

type SparseFileInfo struct {
	utils.DataFileInfo
	size int64
}

func (self SparseFileInfo) Size() int64 {
	return self.size
}

type SparseReader struct {
	mu     sync.Mutex
	offset int64
	size   int64

	// A file handle to the /proc/pid/mem file.
	handle glob.ReadSeekCloser
	ranges []*uploads.Range
}

// Repeat the read operation one page at the time in order to retrieve
// as much data as possible.
func (self *SparseReader) readDistinctPages(buf []byte) (int, error) {
	page_count := len(buf) / PAGE_SIZE
	if page_count <= 1 {
		return page_count * PAGE_SIZE, nil
	}

	// Read as many pages as possible into the buffer ignoring errors.
	for i := 0; i < page_count; i += 1 {
		buf_start := i * PAGE_SIZE
		buf_end := buf_start + PAGE_SIZE

		// Repeat the read with a single page at the time.
		self.handle.Seek(self.offset, os.SEEK_SET)
		_, err := self.handle.Read(buf[buf_start:buf_end])
		if err != nil {
			// Error occured reading a single page, zero
			// it out and skip the page.
			for i := buf_start; i < buf_end; i++ {
				buf[i] = 0
			}
			self.offset += PAGE_SIZE
		}
	}

	return page_count * PAGE_SIZE, nil
}

func (self *SparseReader) Read(buf []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	current_range, next_range := uploads.GetNextRange(self.offset, self.ranges)
	// Current offset is inside the range.
	if current_range != nil {
		to_read := current_range.Offset + current_range.Length - self.offset
		if to_read > int64(len(buf)) {
			to_read = int64(len(buf))
		}

		if current_range.IsSparse {
			for i := int64(0); i < to_read; i++ {
				buf[i] = 0
			}
		} else {
			// Read memory from process at specified offset.
			self.handle.Seek(self.offset, os.SEEK_SET)
			_, err := self.handle.Read(buf[:to_read])

			// A read error occured - split the read into multiple page
			// size reads to get as much data as we can out of the
			// region. Note: We always return as much data as was
			// required, we simply null pad the missing data. Therefore if
			// a reader askes to read from a memory region that contains
			// no data, we never return an error - just zero pad those
			// regions.
			if err != nil {
				return self.readDistinctPages(buf)
			}
		}

		// Advance the read pointer.
		self.offset += to_read

		return int(to_read), nil
	}

	// The current offset is not inside any range so we null pad until
	// the next range.
	if next_range != nil {
		to_read := next_range.Offset - self.offset
		if to_read > int64(len(buf)) {
			to_read = int64(len(buf))
		}

		// Clear the buffer
		for i := range buf[:to_read] {
			buf[i] = 0
		}
		self.offset += to_read
		return int(to_read), nil
	}

	// Range is past the end of file
	return 0, io.EOF
}

func (self *SparseReader) Ranges() []uploads.Range {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []uploads.Range{}
	size := int64(0)
	for _, rng := range self.ranges {
		// Fill in a sparse range if needed
		if rng.Offset > size {
			result = append(result, uploads.Range{
				Offset:   size,
				Length:   rng.Offset - size,
				IsSparse: true,
			})
		}

		// Move the pointer past the end of this range.
		size = rng.Offset + rng.Length

		// Add a real data run
		result = append(result, *rng)
	}
	return result
}

func (self *SparseReader) Seek(offset int64, whence int) (int64, error) {
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

func (self SparseReader) Close() error {
	return self.handle.Close()
}

func (self SparseReader) Stat() (os.FileInfo, error) {
	return &SparseFileInfo{size: self.size}, nil
}

func (self SparseReader) LStat() (glob.FileInfo, error) {
	return &SparseFileInfo{size: self.size}, nil
}

func GetSparseFile(file_path string, scope vfilter.Scope) (ReaderStat, error) {
	pathspec, err := glob.PathSpecFromString(file_path)
	if err != nil {
		return nil, err
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, pathspec.DelegateAccessor)
	if err != nil {
		scope.Log("%v: DelegateAccessor denied", err)
		return nil, err
	}

	accessor, err := glob.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("%v: did you provide a URL or PathSpec?", err)
		return nil, err
	}

	fd, err := accessor.Open(pathspec.GetDelegatePath())
	if err != nil {
		scope.Log("%v: Failed to open delegate", err)
		return nil, err
	}

	// Devices can not be stat'ed
	size := int64(0)
	stat, err := fd.Stat()
	if err == nil {
		size = int64(stat.Size())
	}

	// The Path is a serialized ranges map.
	ranges, err := parseRanges([]byte(pathspec.Path))
	if err != nil {
		scope.Log("Sparse accessor expects ranges as path, for example: '[{Offset:0, Length: 10},{Offset:10,length:20}]'")
		return nil, err
	}

	if size == 0 && len(ranges) > 0 {
		last := ranges[len(ranges)-1]
		size = last.Offset + last.Length
	}

	return &SparseReader{
		handle: fd,
		size:   size,
		ranges: ranges,
	}, nil
}

func init() {
	glob.Register("sparse", &GzipFileSystemAccessor{
		getter: GetSparseFile}, `Allow reading another file by overlaying a sparse map on top of it.

The map excludes reading from certain areas which are considered sparse.

The resulting file is sparse (and therefore uploading it excludes the masked out regions). The filename is taken as a list of ranges. For example:

FileName = pathspec(
      DelegateAccessor="data", DelegatePath=MyData,
      Path=[dict(Offset=0,Length=5), dict(Offset=10,Length=5)])
`)
}
