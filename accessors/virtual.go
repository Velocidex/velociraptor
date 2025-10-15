package accessors

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type VirtualReadSeekCloser struct {
	io.ReadSeeker
}

func (self VirtualReadSeekCloser) Close() error {
	return nil
}

type VirtualFileInfo struct {
	// Only valid when this is not a directory
	RawData []byte

	Data_  *ordereddict.Dict
	IsDir_ bool
	Size_  int64

	Path   *OSPath
	Atime_ time.Time
	Mtime_ time.Time
	Ctime_ time.Time
	Btime_ time.Time
}

func (self *VirtualFileInfo) IsDir() bool {
	return self.IsDir_
}

func (self *VirtualFileInfo) OSPath() *OSPath {
	if self.Path == nil {
		return MustNewGenericOSPath("")
	}
	return self.Path
}

func (self *VirtualFileInfo) Size() int64 {
	if self.Size_ > 0 {
		return self.Size_
	}

	return int64(len(self.RawData))
}

func (self *VirtualFileInfo) Data() *ordereddict.Dict {
	return self.Data_
}

func (self *VirtualFileInfo) Name() string {
	return self.Path.Basename()
}

func (self *VirtualFileInfo) Sys() interface{} {
	return nil
}

func (self *VirtualFileInfo) Mode() os.FileMode {
	if self.IsDir_ {
		return 0755 | os.ModeDir
	}
	return 0644
}

func (self *VirtualFileInfo) ModTime() time.Time {
	return self.Mtime_
}

func (self *VirtualFileInfo) FullPath() string {
	return self.Path.String()
}

func (self *VirtualFileInfo) Btime() time.Time {
	return self.Btime_
}

func (self *VirtualFileInfo) Mtime() time.Time {
	return self.Mtime_
}

func (self *VirtualFileInfo) Ctime() time.Time {
	return self.Ctime_
}

func (self *VirtualFileInfo) Atime() time.Time {
	return self.Atime_
}

func (self *VirtualFileInfo) IsLink() bool {
	return false
}

func (self *VirtualFileInfo) Debug() string {
	return fmt.Sprintf("%v (%v) %s", self.FullPath(), self.Size(), self.Mode())
}

func (self *VirtualFileInfo) GetLink() (*OSPath, error) {
	return nil, errors.New("Not implemented")
}

// Mount tree is very sparse so we dont really need a map here -
// linear search is fast enough.
type directory_node struct {
	file_info *VirtualFileInfo

	// Child directory_nodes
	children []*directory_node
}

func (self *directory_node) Debug() string {
	res := fmt.Sprintf("directory_node: %v\n", json.MustMarshalString(self.file_info))
	for _, c := range self.children {
		res += c.Debug()
	}
	res += "\n"
	return res
}

func (self *directory_node) GetChild(name string) *directory_node {
	for _, c := range self.children {
		if c.file_info != nil &&
			c.file_info.Name() == name {
			return c
		}
	}

	return nil
}

func (self *directory_node) MakeChild(name string) *directory_node {
	if name == "" {
		return self
	}

	for _, c := range self.children {
		if c.file_info != nil &&
			c.file_info.Name() == name {
			return c
		}
	}

	// If we get here there is no child of this name - make it
	new_directory_node := &directory_node{
		file_info: &VirtualFileInfo{
			Path:   self.file_info.OSPath().Append(name),
			IsDir_: true,
		},
	}
	self.children = append(self.children, new_directory_node)
	return new_directory_node
}

// A Virtual Filsystem stores files and directories in memory.
type VirtualFilesystemAccessor struct {
	root_path *OSPath
	root      directory_node
}

func (self VirtualFilesystemAccessor) New(scope vfilter.Scope) (
	FileSystemAccessor, error) {
	return self, nil
}

func (self VirtualFilesystemAccessor) Describe() *AccessorDescriptor {
	return &AccessorDescriptor{
		Name:        "virtual",
		Description: "An accessor for virtual mapped filesystems",
	}
}

func (self VirtualFilesystemAccessor) ParsePath(path string) (*OSPath, error) {
	return self.root.file_info.OSPath().Parse(path)
}

func (self VirtualFilesystemAccessor) Lstat(path string) (FileInfo, error) {
	os_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(os_path)
}

func (self VirtualFilesystemAccessor) LstatWithOSPath(
	path *OSPath) (FileInfo, error) {
	node, err := self.getNode(path)
	if err != nil {
		return nil, err
	}

	return node.file_info, nil
}

func (self VirtualFilesystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	os_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(os_path)
}

func (self VirtualFilesystemAccessor) ReadDirWithOSPath(
	path *OSPath) ([]FileInfo, error) {

	node, err := self.getNode(path)
	if err != nil {
		return nil, err
	}

	result := make([]FileInfo, 0, len(node.children))
	for _, c := range node.children {
		result = append(result, c.file_info)
	}

	return result, nil
}

func (self VirtualFilesystemAccessor) Open(path string) (
	ReadSeekCloser, error) {
	os_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(os_path)
}

func (self VirtualFilesystemAccessor) OpenWithOSPath(path *OSPath) (
	ReadSeekCloser, error) {
	node, err := self.getNode(path)
	if err != nil {
		return nil, utils.NotFoundError
	}

	return VirtualReadSeekCloser{
		ReadSeeker: bytes.NewReader(node.file_info.RawData),
	}, nil
}

func (self VirtualFilesystemAccessor) getNode(path *OSPath) (*directory_node, error) {
	node := &self.root

	for _, c := range path.Components {
		if c != "" {
			next_node := node.GetChild(c)
			if next_node == nil {
				return nil, fmt.Errorf("While finding %v: Can not find %v: %w",
					path, c, utils.NotFoundError)
			}
			node = next_node
		}
	}
	return node, nil
}

func (self *VirtualFilesystemAccessor) SetVirtualDirectory(
	dir_path *OSPath, file_info *VirtualFileInfo) {

	node := &self.root

	for _, c := range dir_path.Components {
		node = node.MakeChild(c)
	}

	file_info.Path = dir_path.Copy()
	node.file_info = file_info
}

func (self *VirtualFilesystemAccessor) SetVirtualFileInfo(
	file_info *VirtualFileInfo) {

	node := &self.root

	for _, c := range file_info.OSPath().Components {
		if c != "" {
			node = node.MakeChild(c)
		}
	}
	node.file_info = file_info
}

func NewVirtualFilesystemAccessor(root_path *OSPath) *VirtualFilesystemAccessor {
	return &VirtualFilesystemAccessor{
		root_path: root_path,
		root: directory_node{
			file_info: &VirtualFileInfo{
				Path:   root_path,
				IsDir_: true,
			},
		},
	}
}

func init() {
	json.RegisterCustomEncoder(&VirtualFileInfo{}, MarshalGlobFileInfo)
}
