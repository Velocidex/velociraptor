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
	return self.Path
}

func (self *VirtualFileInfo) Size() int64 {
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
		return 0755
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
	root directory_node
}

func (self VirtualFilesystemAccessor) New(scope vfilter.Scope) (
	FileSystemAccessor, error) {
	return VirtualFilesystemAccessor{}, nil
}

func (self VirtualFilesystemAccessor) Lstat(filename string) (FileInfo, error) {
	node, err := self.getNode(filename)
	if err != nil {
		return nil, err
	}

	return node.file_info, nil
}

func (self VirtualFilesystemAccessor) ReadDir(path string) ([]FileInfo, error) {
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
	node, err := self.getNode(path)
	if err != nil {
		return nil, os.ErrNotExist
	}

	return VirtualReadSeekCloser{
		ReadSeeker: bytes.NewReader(node.file_info.RawData),
	}, nil
}

func (self VirtualFilesystemAccessor) getNode(path string) (*directory_node, error) {
	node := &self.root

	for _, c := range utils.SplitComponents(path) {
		if c != "" {
			next_node := node.GetChild(c)
			if next_node == nil {
				return nil, os.ErrNotExist
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

	file_info.Path = dir_path
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

func NewVirtualFilesystemAccessor() *VirtualFilesystemAccessor {
	return &VirtualFilesystemAccessor{
		root: directory_node{
			file_info: &VirtualFileInfo{
				Path:   NewLinuxOSPath(""),
				IsDir_: true,
			},
		},
	}
}
