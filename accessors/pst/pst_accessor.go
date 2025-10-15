//go:build !arm && !mips && !(linux && 386)
// +build !arm
// +build !mips
// +build !linux !386

package pst

import (
	"errors"
	"io"
	"strconv"
	"strings"

	pst "github.com/mooijtech/go-pst/v6/pkg"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type ReaderWrapper struct {
	io.ReadSeeker
	closer func()
}

func (self *ReaderWrapper) Close() error {
	self.closer()
	return nil
}

type PSTFileSystemAccessor struct {
	scope vfilter.Scope
}

func (self PSTFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewGenericOSPath(path)
}

func (self PSTFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name: "pst",
		Description: `An accessor to open attachments in PST files.

This accessor allows opening of attachments for scanning or reading.

The OSPath used is structured in the form:

{
  Path: "Msg/<msg_id>/Att/<attach_id>/filename",
  DelegatePath: <path to PST file>,
  DelegateAccessor: <accessor for PST file>
}
`,
		Permissions: []acls.ACL_PERMISSION{acls.FILESYSTEM_READ},
	}
}

func (self PSTFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	return &PSTFileSystemAccessor{scope: scope}, nil
}

func (self PSTFileSystemAccessor) ReadDir(path string) (
	[]accessors.FileInfo, error) {
	return nil, errors.New("Unable to list all MFT entries.")
}

func (self PSTFileSystemAccessor) ReadDirWithOSPath(path *accessors.OSPath) (
	[]accessors.FileInfo, error) {
	return nil, errors.New("Unable to list all MFT entries.")
}

func (self *PSTFileSystemAccessor) Open(path string) (
	accessors.ReadSeekCloser, error) {

	full_path, err := self.ParsePath(path)
	if err != nil || len(full_path.Components) == 0 {
		return nil, utils.NotFoundError
	}

	return self.OpenWithOSPath(full_path)
}

func (self *PSTFileSystemAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {

	attachment, closer, err := self.getAttachment(full_path)
	if err != nil {
		return nil, err
	}

	attachmentReader, err := attachment.PropertyContext.GetPropertyReader(
		14081, attachment.LocalDescriptors)
	if err != nil {
		return nil, err
	}

	return &ReaderWrapper{
		ReadSeeker: io.NewSectionReader(&attachmentReader, 0, attachmentReader.Size()),
		closer:     closer,
	}, nil
}

func (self *PSTFileSystemAccessor) Lstat(path string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(path)
	if err != nil || len(full_path.Components) == 0 {
		return nil, utils.NotFoundError
	}

	return self.LstatWithOSPath(full_path)
}

func (self *PSTFileSystemAccessor) getAttachment(
	path *accessors.OSPath) (res *pst.Attachment, closer func(), err error) {

	if len(path.Components) == 0 {
		return nil, nil, utils.NotFoundError
	}

	filename := path.Components[len(path.Components)-1]
	if !strings.HasPrefix(filename, "Att-") {
		return nil, nil, utils.Wrap(
			utils.NotFoundError, "Invalid Path format for PST accessor")
	}

	att_id, err := strconv.ParseInt(filename[4:], 0, 64)
	if err != nil {
		return nil, nil, utils.Wrap(
			utils.NotFoundError, "Invalid Path format for PST accessor")
	}

	pst_cache := GetPSTCache(self.scope)
	delegate, err := path.Delegate(self.scope)
	if err != nil {
		return nil, nil, err
	}

	pstFile, err := pst_cache.Open(self.scope, path.DelegateAccessor(), delegate)
	if err != nil {
		return nil, nil, err
	}

	return pstFile.GetAttachment(pst.Identifier(att_id))
}

func (self *PSTFileSystemAccessor) LstatWithOSPath(full_path *accessors.OSPath) (
	accessors.FileInfo, error) {

	attachment, closer, err := self.getAttachment(full_path)
	if err != nil {
		return nil, err
	}
	defer closer()

	attachmentReader, err := attachment.PropertyContext.GetPropertyReader(
		14081, attachment.LocalDescriptors)
	if err != nil {
		return nil, err
	}

	return &accessors.VirtualFileInfo{
		Size_: attachmentReader.Size(),
		Path:  full_path,
	}, nil
}

func init() {
	accessors.Register(&PSTFileSystemAccessor{})
}
