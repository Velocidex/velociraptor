package smb

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"github.com/hirochachacha/go-smb2"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	SMB_TAG = "$SMB_CACHE"
)

type SMBAccessorArgs struct {
	Hosts *ordereddict.Dict `vfilter:"required,field=hosts,doc=A dict mapping hostname to connection strings. The connection string consists of username and password joined by colon (e.g. fred:hunter2 )."`
}

// Real implementation for non windows OSs:
type SMBFileSystemAccessor struct {
	root *accessors.OSPath

	scope vfilter.Scope
}

func (self *SMBFileSystemAccessor) ParsePath(path string) (*accessors.OSPath, error) {
	return self.root.Parse(path)
}

func (self SMBFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "smb",
		Description: `Allows access to SMB shares.`,
		Permissions: []acls.ACL_PERMISSION{acls.NETWORK},
		ScopeVar:    constants.SMB_CREDENTIALS,
		ArgType:     &SMBAccessorArgs{},
	}
}

func (self *SMBFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	return &SMBFileSystemAccessor{
		root:  self.root,
		scope: scope,
	}, nil
}

func (self *SMBFileSystemAccessor) Lstat(filename string) (accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self *SMBFileSystemAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {

	fs, directory, closer, err := self.getMount(full_path)
	if err != nil {
		return nil, err
	}
	defer closer()

	lstat, err := fs.Lstat(directory)
	if err != nil {
		return nil, err
	}

	return makeFileInfo(lstat, full_path), nil
}

func (self *SMBFileSystemAccessor) getSession(full_path *accessors.OSPath) (
	*smb2.Session, func(), error) {
	if len(full_path.Components) == 0 {
		return nil, nil, errors.New("First path component for smb accessor must be a server name or IP")
	}

	cache, pres := vql_subsystem.CacheGet(self.scope, SMB_TAG).(*SMBMountCache)
	if !pres {
		cache = NewSMBMountCache(self.scope)
		vql_subsystem.CacheSet(self.scope, SMB_TAG, cache)
	}

	server_name := full_path.Components[0]
	connection, closer, err := cache.GetHandle(server_name)
	if err != nil {
		return nil, nil, err
	}

	return connection.Session(), closer, nil
}

func (self *SMBFileSystemAccessor) getMount(full_path *accessors.OSPath) (
	*smb2.Share, string, func(), error) {
	if len(full_path.Components) < 2 {
		return nil, "", nil, errors.New("SMBFileSystemAccessor.LstatWithOSPath requires at least a server name and share name.")
	}

	session, closer, err := self.getSession(full_path)
	if err != nil {
		return nil, "", nil, err
	}

	share := full_path.Components[1]
	fs, err := session.Mount(share)
	if err != nil {
		return nil, "", nil, err
	}

	directory := "."
	if len(full_path.Components) > 2 {
		directory = strings.Join(full_path.Components[2:], "\\")
	}

	return fs, directory, closer, nil
}

func (self *SMBFileSystemAccessor) ReadDir(dir string) ([]accessors.FileInfo, error) {
	full_path, err := self.root.Parse(dir)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self *SMBFileSystemAccessor) listShares(
	full_path *accessors.OSPath) ([]accessors.FileInfo, error) {

	session, closer, err := self.getSession(full_path)
	if err != nil {
		return nil, err
	}
	defer closer()

	names, err := session.ListSharenames()
	if err != nil {
		return nil, err
	}

	var result []accessors.FileInfo
	for _, name := range names {
		finfo := &accessors.VirtualFileInfo{
			Path:   full_path.Append(name),
			IsDir_: true,
		}
		result = append(result, finfo)
	}
	return result, nil
}

func (self *SMBFileSystemAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) ([]accessors.FileInfo, error) {
	if len(full_path.Components) == 0 {
		return nil, errors.New("First path component for smb accessor must be a server name or IP")
	}

	// If we only have a server name, it means to list the shares
	if len(full_path.Components) == 1 {
		return self.listShares(full_path)
	}

	cache, pres := vql_subsystem.CacheGet(self.scope, SMB_TAG).(*SMBMountCache)
	if !pres {
		cache = NewSMBMountCache(self.scope)
		vql_subsystem.CacheSet(self.scope, SMB_TAG, cache)
	}

	server_name := full_path.Components[0]
	connection, closer, err := cache.GetHandle(server_name)
	if err != nil {
		return nil, err
	}
	defer closer()

	share := full_path.Components[1]
	fs, err := connection.Mount(share)
	if err != nil {
		return nil, err
	}

	directory := "."
	if len(full_path.Components) > 2 {
		directory = strings.Join(full_path.Components[2:], "\\")
	}

	matches, err := fs.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	var result []accessors.FileInfo
	for _, match := range matches {
		result = append(result, makeFileInfo(
			match, full_path.Append(match.Name())))
	}
	return result, nil
}

func makeFileInfo(finfo fs.FileInfo,
	full_path *accessors.OSPath) *accessors.VirtualFileInfo {
	result := &accessors.VirtualFileInfo{
		IsDir_: finfo.IsDir(),
		Size_:  finfo.Size(),
		Path:   full_path,
		Mtime_: finfo.ModTime(),
	}

	sys, ok := finfo.Sys().(*smb2.FileStat)
	if ok {
		result.Atime_ = sys.LastAccessTime
		result.Ctime_ = sys.ChangeTime
		result.Btime_ = sys.CreationTime
	}
	return result
}

// Wrap the os.File object to keep track of open file handles.
type SMBFileWrapper struct {
	*smb2.File
	closed bool
}

func (self *SMBFileWrapper) DebugString() string {
	return fmt.Sprintf("SMBFileWrapper %v (closed %v)", self.Name(), self.closed)
}

func (self *SMBFileWrapper) Close() error {
	smbAccessorCurrentOpened.Dec()
	self.closed = true
	return self.File.Close()
}

func (self *SMBFileSystemAccessor) Open(path string) (accessors.ReadSeekCloser, error) {
	// Clean the path
	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *SMBFileSystemAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	fs, path, closer, err := self.getMount(full_path)
	if err != nil {
		return nil, err
	}
	defer closer()

	file_obj, err := fs.Open(path)
	if err != nil {
		return nil, err
	}

	smbAccessorCurrentOpened.Inc()
	return &SMBFileWrapper{File: file_obj}, nil
}

func init() {
	root_path := &accessors.OSPath{
		Manipulator: &SMBPathManipulator{},
	}
	accessors.Register(&SMBFileSystemAccessor{
		root: root_path,
	})
}
