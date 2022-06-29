package collector

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"strings"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/zip"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// An accessor for reading collector containers. The Offline collector
// is designed to store files in a zip container. However the zip
// format is not capable of storing certain attributes (e.g. sparse
// files).  The accessor is designed to read the offline collector
// containers.

type rangedReader struct {
	delegate io.ReaderAt
	fd       accessors.ReadSeekCloser
	offset   int64
}

func (self *rangedReader) Read(buff []byte) (int, error) {
	n, err := self.delegate.ReadAt(buff, self.offset)
	self.offset += int64(n)
	return n, err
}

func (self *rangedReader) Seek(offset int64, whence int) (int64, error) {
	self.offset = offset
	return self.offset, nil
}

func (self *rangedReader) Close() error {
	return self.fd.Close()
}

type StatWrapper struct {
	accessors.FileInfo
	real_size int64
}

func (self StatWrapper) Size() int64 {
	return self.real_size
}

type CollectorAccessor struct {
	*zip.ZipFileSystemAccessor
	scope vfilter.Scope
}

func (self *CollectorAccessor) New(scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	delegate, err := (&zip.ZipFileSystemAccessor{}).New(scope)
	return &CollectorAccessor{
		ZipFileSystemAccessor: delegate.(*zip.ZipFileSystemAccessor),
		scope:                 scope,
	}, err
}

func (self *CollectorAccessor) Open(
	filename string) (accessors.ReadSeekCloser, error) {

	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *CollectorAccessor) getIndex(
	full_path *accessors.OSPath) (*actions_proto.Index, error) {

	// Does the file have an idx file?
	idx_reader, err := self.ZipFileSystemAccessor.OpenWithOSPath(
		full_path.Dirname().Append(full_path.Basename() + ".idx"))
	if err != nil {
		return nil, err
	}

	serialized, err := ioutil.ReadAll(idx_reader)
	if err != nil {
		return nil, err
	}

	index := &actions_proto.Index{}
	err = json.Unmarshal(serialized, index)
	if err != nil {
		// Older versions stored idx as a JSONL file instead.
		for _, l := range strings.Split(string(serialized), "\n") {
			if len(l) > 2 {
				r := &actions_proto.Range{}
				err = json.Unmarshal([]byte(l), r)
				if err != nil {
					return nil, err
				}
				index.Ranges = append(index.Ranges, r)
			}
		}
	}

	if len(index.Ranges) == 0 {
		return nil, errors.New("No ranges")
	}

	return index, err
}

func (self *CollectorAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	reader, err := self.ZipFileSystemAccessor.OpenWithOSPath(full_path)
	if err != nil {
		return nil, err
	}

	index, err := self.getIndex(full_path)
	if err == nil {
		return &rangedReader{
			delegate: &utils.RangedReader{
				ReaderAt: utils.ReaderAtter{Reader: reader},
				Index:    index,
			},
			fd: reader,
		}, nil
	}

	return reader, nil
}

func (self *CollectorAccessor) Lstat(file_path string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(file_path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self *CollectorAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {

	stat, err := self.ZipFileSystemAccessor.LstatWithOSPath(full_path)
	if err != nil {
		return nil, err
	}

	index, err1 := self.getIndex(full_path)
	if err1 == nil {
		real_size := int64(0)
		for _, r := range index.Ranges {
			real_size = r.OriginalOffset + r.Length
		}

		return StatWrapper{
			FileInfo:  stat,
			real_size: real_size,
		}, nil
	}

	return stat, err
}

func init() {
	accessors.Register("collector", &CollectorAccessor{},
		`Open a collector zip file as if it was a directory.`)
}
