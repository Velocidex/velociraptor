package ssh

import (
	"errors"
	"strings"

	"github.com/pkg/sftp"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	ErrNotFound     = errors.New("file not found")
	ErrNotAvailable = errors.New("File content not available")
)

type SSHFileSystemAccessor struct {
	scope       vfilter.Scope
	sftp_client *sftp.Client
}

func (self SSHFileSystemAccessor) New(scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	ssh_client, closer, err := GetSSHClient(scope)
	if err != nil {
		return nil, err
	}

	sftp_client, err := sftp.NewClient(ssh_client)
	if err != nil {
		ssh_client.Close()
		return nil, err
	}

	// Close the ssh client when the scope destroys.
	err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
		sftp_client.Close()
		_ = closer()
	})
	if err != nil {
		sftp_client.Close()
		_ = closer()
		return nil, err
	}

	return &SSHFileSystemAccessor{
		scope:       scope,
		sftp_client: sftp_client,
	}, nil
}

func (self SSHFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "ssh",
		Description: `Access a remote system's filesystem via SSH/SFTP.`,
		Permissions: []acls.ACL_PERMISSION{acls.NETWORK},
		ScopeVar:    constants.SSH_CONFIG,
		ArgType:     &SSHAccessorArgs{},
	}
}

func (self SSHFileSystemAccessor) Lstat(filename string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self SSHFileSystemAccessor) LstatWithOSPath(filename *accessors.OSPath) (
	accessors.FileInfo, error) {

	path := "/" + strings.Join(filename.Components, "/")
	info, err := self.sftp_client.Lstat(path)
	if err != nil {
		return nil, err
	}

	return NewSFTPFileInfo(info, filename), nil
}

func (self SSHFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewGenericOSPath(path)
}

func (self SSHFileSystemAccessor) ReadDir(filename string) (
	[]accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self SSHFileSystemAccessor) ReadDirWithOSPath(
	filename *accessors.OSPath) (
	result []accessors.FileInfo, err error) {

	path := "/" + strings.Join(filename.Components, "/")
	dir, err := self.sftp_client.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, d := range dir {
		child := filename.Append(d.Name())
		result = append(result, NewSFTPFileInfo(d, child))
	}

	return result, err
}

func (self SSHFileSystemAccessor) Open(filename string) (
	accessors.ReadSeekCloser, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self SSHFileSystemAccessor) OpenWithOSPath(filename *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {

	path := "/" + strings.Join(filename.Components, "/")
	fd, err := self.sftp_client.Open(path)
	if err != nil {
		return nil, err
	}

	return &SFTPFileWrapper{fd}, nil
}

func init() {
	accessors.Register(&SSHFileSystemAccessor{})
}
