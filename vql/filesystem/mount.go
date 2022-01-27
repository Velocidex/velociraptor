package filesystem

import (
	"errors"
	"path/filepath"

	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/vfilter"
)

type MountFileSystemAccessor struct {
	scope vfilter.Scope
}

func (self MountFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	return &MountFileSystemAccessor{scope: scope}, nil
}

func (self *MountFileSystemAccessor) ensureBackingAccessor(pathSpec *glob.PathSpec) (glob.FileSystemAccessor, error) {
	if pathSpec.GetDelegateAccessor() == "" {
		return nil, errors.New("no delegate accessor specified")
	}
	accessor, err := glob.GetAccessor(pathSpec.GetDelegateAccessor(), self.scope)
	if err != nil {
		return nil, err
	}
	return accessor, nil
}

func (self *MountFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	pathSpec, err := glob.PathSpecFromString(path)
	if err != nil {
		return nil, err
	}

	accessor, err := self.ensureBackingAccessor(pathSpec)
	if err != nil {
		return nil, err
	}

	return accessor.ReadDir(path)
}

func (self *MountFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	pathSpec, err := glob.PathSpecFromString(path)
	if err != nil {
		return nil, err
	}

	accessor, err := self.ensureBackingAccessor(pathSpec)
	if err != nil {
		return nil, err
	}

	return accessor.Open(path)
}

func (self MountFileSystemAccessor) Lstat(filename string) (glob.FileInfo, error) {
	pathSpec, err := glob.PathSpecFromString(filename)
	if err != nil {
		return nil, err
	}

	accessor, err := self.ensureBackingAccessor(pathSpec)
	if err != nil {
		return nil, err
	}

	return accessor.Lstat(filename)
}

func (self *MountFileSystemAccessor) PathSplit(path string) []string {
	pathSpec, err := glob.PathSpecFromString(path)
	if err != nil {
		return paths.GenericPathSplit(path)
	}

	accessor, err := self.ensureBackingAccessor(pathSpec)
	if err != nil {
		return paths.GenericPathSplit(path)
	}

	return accessor.PathSplit(pathSpec.GetPath())
}

func (self *MountFileSystemAccessor) PathJoin(root, stem string) string {
	pathSpec, err := glob.PathSpecFromString(root)
	if err != nil {
		return filepath.Join(root, stem)
	}

	accessor, err := self.ensureBackingAccessor(pathSpec)
	if err != nil {
		return filepath.Join(root, stem)
	}

	pathSpec.Path = accessor.PathJoin(pathSpec.Path, stem)

	return pathSpec.String()
}

func (self *MountFileSystemAccessor) GetRoot(path string) (root, subpath string, err error) {
	pathSpec, err := glob.PathSpecFromString(path)
	if err != nil {
		return "", "", err
	}

	accessor, err := self.ensureBackingAccessor(pathSpec)
	if err != nil {
		return "", "", err
	}

	return accessor.GetRoot(pathSpec.GetPath())
}

func init() {
	glob.Register("mount", &MountFileSystemAccessor{}, `Accessor which emulates mounting another resource as the root of the filesystem used by the glob.`)
}
