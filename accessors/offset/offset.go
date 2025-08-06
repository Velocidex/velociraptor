// An accessor that maps ranges from a delegate.

package offset

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/zip"
	"www.velocidex.com/golang/vfilter"
)

type OffsetFileInfo struct {
	accessors.FileInfo

	_full_path *accessors.OSPath
}

func (self *OffsetFileInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

type OffsetReader struct {
	reader io.ReadSeekCloser
	info   accessors.FileInfo

	// Current offset of the reader in delegate coordinates
	offset int64

	// Constant offset we add the the delegate reader.
	base_offset int64

	// The OSPath object that is required to access this
	// file. (includes delegate and offset).
	_full_path *accessors.OSPath
}

func (self *OffsetReader) Close() error {
	return self.reader.Close()
}

func (self *OffsetReader) Read(buff []byte) (int, error) {
	new_pos, err := self.reader.Seek(self.offset, os.SEEK_SET)
	if err != nil {
		return int(new_pos), err
	}

	n, err := self.reader.Read(buff)
	self.offset += int64(n)
	return n, err
}

func (self *OffsetReader) Seek(offset int64, whence int) (int64, error) {

	// Callers are operating in the offsetted coordinate system so the
	// real offset should be the offset they asked for plus the base
	// offset.
	if whence == os.SEEK_SET {
		offset += self.base_offset
	}

	new_delegate_offset, err := self.reader.Seek(offset, whence)
	if err != nil {
		return new_delegate_offset, err
	}

	// Remember the delegate offset
	self.offset = new_delegate_offset

	// Report the new offset in terms of the offsetted coordinate.
	return self.offset - self.base_offset, nil
}

func (self *OffsetReader) LStat() (accessors.FileInfo, error) {
	return &OffsetFileInfo{
		FileInfo:   self.info,
		_full_path: self._full_path,
	}, nil
}

func GetOffsetFile(full_path *accessors.OSPath, scope vfilter.Scope) (
	zip.ReaderStat, error) {

	if len(full_path.Components) == 0 {
		return nil, fmt.Errorf("Offset accessor expects an offset at root path")

	}

	offset, err := strconv.ParseInt(full_path.Components[0], 0, 64)
	if err != nil {
		return nil, fmt.Errorf("Offset accessor expects an offset path: %w", err)
	}

	pathspec := full_path.PathSpec()

	// The gzip accessor must use a delegate but if one is not
	// provided we use the "auto" accessor, to open the underlying
	// file.
	if pathspec.DelegateAccessor == "" && pathspec.GetDelegatePath() == "" {
		pathspec.DelegatePath = pathspec.Path
		pathspec.DelegateAccessor = "auto"
	}

	accessor, err := accessors.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("%v: did you provide a URL or PathSpec?", err)
		return nil, err
	}

	delegate_path := pathspec.GetDelegatePath()
	fd, err := accessor.Open(delegate_path)
	if err != nil {
		return nil, err
	}

	stat, err := accessor.Lstat(delegate_path)
	if err != nil {
		// If we can not call stat on the file it is not a fatal
		// error. For example, raw files are not always statable - in
		// that case we provide a fake stat object.
		stat = &accessors.VirtualFileInfo{
			Path:  full_path,
			Size_: 1<<63 - 1,
		}
	}

	return &OffsetReader{
		reader:      fd,
		info:        stat,
		offset:      offset,
		base_offset: offset,
		_full_path:  full_path,
	}, nil
}

func init() {
	accessors.Register(accessors.DescribeAccessor(
		zip.NewGzipFileSystemAccessor(
			accessors.MustNewLinuxOSPath(""), GetOffsetFile),
		accessors.AccessorDescriptor{
			Name:        "offset",
			Description: `Allow reading another file from a specific offset.`,
		}))
}
