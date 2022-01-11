package filesystem

import (
	"path"
	"regexp"

	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type MountFileSystemAccessor struct {
	scope vfilter.Scope
}

func (self MountFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	return &MountFileSystemAccessor{scope: scope}, nil
}

func (self *MountFileSystemAccessor) ensureBackingAccessor(pathSpec *glob.PathSpec) (glob.FileSystemAccessor, error) {
	accessor, err := glob.GetAccessor(pathSpec.DelegateAccessor, self.scope)
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

var MountFileSystemAccessorSplit_re = regexp.MustCompile(`[\\/]`)

func (self *MountFileSystemAccessor) PathSplit(path string) []string {
	pathSpec, err := glob.PathSpecFromString(path)
	if err != nil {
		return MountFileSystemAccessorSplit_re.Split(path, -1)
	}

	accessor, err := self.ensureBackingAccessor(pathSpec)
	if err != nil {
		return nil
	}

	return accessor.PathSplit(path)
}

func (self *MountFileSystemAccessor) PathJoin(root, stem string) string {
	pathSpec, err := glob.PathSpecFromString(root)
	if err != nil {
		return path.Join(root, stem)
	}

	accessor, err := self.ensureBackingAccessor(pathSpec)
	if err != nil {
		return path.Join(root, stem)
	}

	return accessor.PathJoin(root, stem)
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

	return accessor.GetRoot(path)
}

func init() {
	glob.Register("mount", &MountFileSystemAccessor{}, `Accessor which emulates mounting another resource as the root of the filesystem used by the glob.`)
}
