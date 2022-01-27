package remapping

import (
	"fmt"
	"strings"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

// Mount tree is very sparse so we dont really need a map here -
// linear search is fast enough.
type node struct {
	// The name of this node in the directory tree.
	name string

	// A path prefix to apply when accessing the accessor
	prefix   string
	accessor glob.FileSystemAccessor

	// Child nodes
	children []*node
}

func (self *node) Debug() string {
	res := fmt.Sprintf("node: %v, prefix: %v, accessor: %T\n",
		self.name, self.prefix, self.accessor)
	for _, c := range self.children {
		res += fmt.Sprintf("  %v\n", strings.Replace(c.Debug(), "\n", "  \n", -1))
	}
	return res
}

func (self *node) GetChild(name string) *node {
	for _, c := range self.children {
		if c.name == name {
			return c
		}
	}

	return nil
}

func (self *node) MakeChild(name string) *node {
	for _, c := range self.children {
		if c.name == name {
			return c
		}
	}

	// If we get here there is no child of this name - make it
	new_node := &node{
		name: name,
	}
	self.children = append(self.children, new_node)
	return new_node
}

// Our delegate accessors deal with real full paths but we want to
// pretend they are mounted inside their respective prefixes,
// therefore we need to wrap them to return the correct virtual
// fullpath.
type FileInfoWrapper struct {
	glob.FileInfo
	prefix string
}

func (self FileInfoWrapper) FullPath() string {
	return strings.TrimPrefix(self.FileInfo.FullPath(), self.prefix)
}

// A mount accessor maps several delegate accessors inside the same
// filesystem tree emulating mount points.
type MountFileSystemAccessor struct {
	mu    sync.Mutex
	scope vfilter.Scope

	// The root filesystem is the one registered
	root *node
}

// Walk the tree and return the last valid node that can be used to
// access the delegates as well as the residual path.
// Example:
// /usr is mounted on /mnt/ - therefore node tree will look like:
// root -> prefix: /, accessor: file, children: [
//   node: name: usr, accessor: file, prefix: /mnt/data,
// ]
//
// Now assume we accessor /usr/bin/ -> We walk the tree from root:
// 1. First component is usr -> next node is child root's child.
// 2. The residual is the rest of the path which is not consumed yet -> i.e. "bin"
// 3. Now, we can access the file as node.prefix + residual -> /mnt/data/bin/
func (self *MountFileSystemAccessor) getDelegateNode(path string) (
	*node, string) {
	node := self.root
	components := self.root.accessor.PathSplit(path)

	for idx, c := range components {
		if c != "" {
			next_node := node.GetChild(c)
			if next_node == nil {
				residual := ""
				for i := idx; i < len(components); i++ {
					residual = self.root.accessor.PathJoin(residual, components[i])
				}
				return node, residual
			}
		}
	}
	return node, path
}

func (self *MountFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	return &MountFileSystemAccessor{scope: scope, root: self.root}, nil
}

func (self *MountFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	delegate_node, residual := self.getDelegateNode(path)
	delegate_path := delegate_node.accessor.PathJoin(delegate_node.prefix, residual)
	children, err := delegate_node.accessor.ReadDir(delegate_path)
	if err != nil {
		return nil, err
	}

	res := make([]glob.FileInfo, 0, len(children))
	for _, c := range children {
		res = append(res, &FileInfoWrapper{
			FileInfo: c,
			prefix:   delegate_node.prefix,
		})
	}

	return res, nil
}

func (self *MountFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	delegate_node, residual := self.getDelegateNode(path)
	delegate_path := delegate_node.accessor.PathJoin(delegate_node.prefix, residual)
	return delegate_node.accessor.Open(delegate_path)
}

func (self MountFileSystemAccessor) Lstat(filename string) (glob.FileInfo, error) {
	delegate_node, residual := self.getDelegateNode(filename)
	delegate_path := delegate_node.accessor.PathJoin(delegate_node.prefix, residual)
	return delegate_node.accessor.Lstat(delegate_path)
}

// The following path manipulation methods just use the root
// accessor's behavior as it guides out path handling.
func (self *MountFileSystemAccessor) PathSplit(path string) []string {
	return self.root.accessor.PathSplit(path)
}

func (self *MountFileSystemAccessor) PathJoin(root, stem string) string {
	return self.root.accessor.PathJoin(root, stem)
}

func (self *MountFileSystemAccessor) GetRoot(path string) (
	root, subpath string, err error) {
	return self.root.accessor.GetRoot(path)
}

// Install a mapping from the source to the target. This means that
// operating on paths below the target will act on the
// source. Examples:
//
// source = /mnt/bin, target = /bin, accessor = file
// means Open(/bin/foo) redirects to /mnt/bin/foo with accessor "file".

func (self *MountFileSystemAccessor) AddMapping(
	source string,
	target string,
	source_accessor glob.FileSystemAccessor) {

	// Walk the tree and create the sentinal node. NOTE: split the
	// path according to the target accessor we are emulating.
	target_components := self.root.accessor.PathSplit(target)
	node := self.root

	for _, c := range target_components {
		if c != "" {
			node = node.MakeChild(c)
		}
	}

	// Install the node in the tree - this is where we read from.
	node.prefix = source
	node.accessor = source_accessor
}

func InstallMountPoint(manager glob.DeviceManager,
	remapping *config_proto.RemappingConfig) error {

	target_accessor_any, err := manager.GetAccessor(remapping.ToAccessor, nil)
	if err != nil {
		return err
	}

	target_accessor, ok := target_accessor_any.(*MountFileSystemAccessor)
	if !ok {
		// The target accessor is not a MountFileSystemAccessor, we
		// neeed to wrap it and in one.
		target_accessor = &MountFileSystemAccessor{
			root: &node{
				accessor: target_accessor_any,
			},
		}
	}

	// We need to get the source accessor from the unmodified device
	// manager (otherwise we will get into a loop here). For example,
	// if we read from "file" and map "file" then the source is the
	// unmodified original file.
	source_accessor, err := glob.GlobalDeviceManager.GetAccessor(
		remapping.FromAccessor, nil)
	if err != nil {
		return err
	}

	// Now target_accessor is valid and of type
	// MountFileSystemAccessor - we can add the mapping
	target_accessor.AddMapping(
		remapping.FromPrefix, remapping.ToPrefix, source_accessor)

	// Replace the target accessor with the remapped one.
	manager.Register(remapping.ToAccessor, target_accessor, "")

	return nil
}
