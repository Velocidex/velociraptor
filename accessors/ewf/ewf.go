package ewf

// An accessor that opens an EWF image
import (
	"errors"
	"io"
	"os"

	"github.com/Velocidex/go-ewf/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/zip"
	"www.velocidex.com/golang/vfilter"
)

type EWFReader struct {
	readers []io.ReadSeekCloser

	offset int64
	ewf    *parser.EWFFile
}

func (self *EWFReader) Copy() *EWFReader {
	return &EWFReader{
		readers: self.readers,
		ewf:     self.ewf,
	}
}

// Files will only be closed when the scope is destroyed. This is ok
// because we do not normally have too many EWF files open in the same
// query.
func (self *EWFReader) _ReallyClose() {
	for _, r := range self.readers {
		r.Close()
	}
}

func (self *EWFReader) Close() error {
	return nil
}

func (self *EWFReader) Read(buff []byte) (int, error) {
	n, err := self.ewf.ReadAt(buff, self.offset)
	if err != nil {
		return 0, err
	}

	if n == 0 {
		return 0, io.EOF
	}

	self.offset += int64(n)
	return n, err
}

func (self *EWFReader) Seek(offset int64, whence int) (int64, error) {
	if whence == os.SEEK_SET {
		self.offset = offset
	} else if whence == os.SEEK_CUR {
		self.offset += offset
	}
	return self.offset, nil
}

func (self *EWFReader) LStat() (accessors.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func GetEWFImage(full_path *accessors.OSPath, scope vfilter.Scope) (
	zip.ReaderStat, error) {

	pathspec := full_path.PathSpec()

	// The EWF accessor must use a delegate but if one is not
	// provided we use the "auto" accessor, to open the underlying
	// file.
	if pathspec.DelegateAccessor == "" && pathspec.GetDelegatePath() == "" {
		pathspec.DelegatePath = pathspec.Path
		pathspec.DelegateAccessor = "auto"
		pathspec.Path = "/"
		err := full_path.SetPathSpec(pathspec)
		if err != nil {
			return nil, err
		}
	}

	accessor, err := accessors.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("ewf: %v: did you provide a DelegateAccessor PathSpec?", err)
		return nil, err
	}

	return getCachedEWFFile(full_path, accessor, scope)
}

func init() {
	accessors.Register(accessors.DescribeAccessor(
		zip.NewGzipFileSystemAccessor(
			accessors.MustNewLinuxOSPath(""), GetEWFImage),
		accessors.AccessorDescriptor{
			Name:        "ewf",
			Description: `Allow reading an EWF file.`,
		}))
}
