package accessors

/*
  The mount accessor represents a filesystem built by combining other
  filesystems in the same tree - i.e. "mounting" them.

  It is used to redirect various directories into multiple different
  accessors.

  NOTE: Currently it is required that filesystems are mounted on
  directories that exist within the containing filesystem: For example
  if mounting an accessor on /usr/bin it is required that /usr/bin
  exist in the root filesystem.
*/

import (
	"fmt"
	"strings"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// Mount tree is very sparse so we dont really need a map here -
// linear search is fast enough.

// The mount accessor is essentially a redirector - it needs to find a
// delegate accessor to forward all requests to. In order to determine
// the correct delegate we walk the mount tree from the root. At each
// point in the tree we have an accessor and a prefix to prepend to
// the delegate path.

// For example, supposed a /bin/ filesystem is mounted on /usr/. We
// have the following tree:
// root -> children = [{node: name="bin", prefix="", accessor=bin_fs_accessor]

// To find the path /usr/bin/ls, we walk the tree from the root, find /usr/
// as the top most delegate. However the path we need is /usr/bin/ls,
// therefore we need to access the delegate with prefix + /bin/ls
type node struct {
	// The name of this node in the directory tree.
	name string

	// Components of the full path in the tree from the root.
	path *OSPath

	// A path prefix to apply when accessing the accessor. This allows
	// us to attach a sub directory of the mounted filesystem (like a
	// bind mount).
	prefix *OSPath

	// The accessor to use to access.
	accessor FileSystemAccessor

	// A pointer to the last mount point with an accessor. Used to
	// pre-calculate the prefix and accessor for fast access.
	last_mount_point *node

	// Child nodes
	children []*node
}

func (self *node) Debug() string {
	res := fmt.Sprintf("node: %v, prefix: %v, accessor: %T\n",
		self.name, self.prefix.String(), self.accessor)
	for _, c := range self.children {
		res += fmt.Sprintf("  %v\n", strings.ReplaceAll(c.Debug(), "\n", "  \n"))
	}
	return res
}

// Lookup a child by name. If not found returns nil
func (self *node) GetChild(name string) *node {
	for _, c := range self.children {
		if c.name == name {
			return c
		}
	}

	return nil
}

// Get the child node for the given name. If the node is not found, we
// create a new node based on our last mount point.
func (self *node) MakeChild(name string) *node {
	for _, c := range self.children {
		if c.name == name {
			return c
		}
	}

	// If we get here there is no child of this name - make it based
	// on the last_mount_point.

	// The full path of the new node can be derived from our own full path
	new_node := &node{
		name: name,
		path: self.path.Append(name),

		// This is a link up the directory tree to the last mounted
		// accessor.
		last_mount_point: self.last_mount_point,
		accessor:         self.last_mount_point.accessor,

		// The prefix to prepend to the mounted accessor is derived
		// from our own prefix.
		prefix: self.prefix.Append(name),
	}
	self.children = append(self.children, new_node)
	return new_node
}

// Our delegate accessors deal with real full paths but we want to
// pretend they are mounted inside their respective prefixes,
// therefore we need to wrap them to return the correct virtual
// fullpath.
type FileInfoWrapper struct {
	FileInfo

	// This prefix will be added to all children - it reflects the
	// mount path.
	prefix        *OSPath
	remove_prefix *OSPath

	_ospath *OSPath
}

func (self FileInfoWrapper) FullPath() string {
	return self.OSPath().String()
}

func (self FileInfoWrapper) Name() string {
	return self.OSPath().Basename()
}

func (self FileInfoWrapper) OSPath() *OSPath {
	if self._ospath != nil {
		return self._ospath
	}

	delegate_path := self.FileInfo.OSPath()
	trimmed_path := delegate_path
	if self.remove_prefix != nil {
		trimmed_path = delegate_path.TrimComponents(
			self.remove_prefix.Components...)
	}

	self._ospath = self.prefix.Append(trimmed_path.Components...)
	return self._ospath
}

func NewFileInfoWrapper(fsinfo FileInfo,
	prefix, remove_prefix *OSPath) *FileInfoWrapper {
	return &FileInfoWrapper{
		FileInfo:      fsinfo,
		prefix:        prefix,
		remove_prefix: remove_prefix,
	}
}

// A mount accessor maps several delegate accessors inside the same
// filesystem tree emulating mount points.
type MountFileSystemAccessor struct {
	scope vfilter.Scope

	// The root filesystem is the one registered
	root *node
}

func (self *MountFileSystemAccessor) ParsePath(path string) (*OSPath, error) {
	return self.root.path.Parse(path)
}

// Walk the tree and return the last valid node that can be used to
// access the delegates as well as the residual path.
// Example:
// /usr is mounted on /mnt/ - therefore node tree will look like:
// root -> prefix: /, accessor: file, children: [
//
//	node: name: usr, accessor: file, prefix: /mnt/data,
//
// ]
//
// Now assume we access /usr/bin/ls -> We walk the tree from root:
//  1. First component is usr -> next node is child root's child.
//  2. The residual is the rest of the path which is not consumed yet
//     -> i.e. "bin/ls"
//  3. Now, we can access the file as node.prefix + residual -> /mnt/data/bin/ls
func (self *MountFileSystemAccessor) getDelegateNode(os_path *OSPath) (
	*node, []string, error) {
	node := self.root

	for idx, c := range os_path.Components {
		if c != "" {
			next_node := node.GetChild(c)

			// There is no internal mount point, use the last known
			// mounted filesystem.
			if next_node == nil {
				residual := os_path.Components[idx:]
				return node, residual, nil
			}

			// Search deeper for a better mount point.
			node = next_node
		}
	}
	return node, nil, nil
}

func (self MountFileSystemAccessor) Describe() *AccessorDescriptor {
	return &AccessorDescriptor{}
}

func (self *MountFileSystemAccessor) New(scope vfilter.Scope) (FileSystemAccessor, error) {
	return &MountFileSystemAccessor{
		scope: scope,
		root:  self.root,
	}, nil
}

func (self *MountFileSystemAccessor) ReadDir(path string) (
	[]FileInfo, error) {
	// Parse the path into an OSPath
	os_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(os_path)
}

func (self *MountFileSystemAccessor) ReadDirWithOSPath(os_path *OSPath) (
	[]FileInfo, error) {

	// delegate_node is the node we must list to get this os_path
	// delegate_path is the path we must list in the node to get this os_path
	delegate_node, delegate_path, err := self.getDelegatePath(os_path)
	if err != nil {
		return nil, err
	}
	children, err := delegate_node.accessor.ReadDirWithOSPath(delegate_path)
	if err != nil {
		return nil, err
	}

	res := make([]FileInfo, 0, len(children))
	names := make([]string, 0, len(children))
	for _, c := range children {
		names = append(names, c.Name())
		res = append(res, &FileInfoWrapper{
			FileInfo:      c,
			prefix:        delegate_node.path.Copy(),
			remove_prefix: delegate_node.prefix.Copy(),
		})
	}

	// If we are listing the path of the delegate node, we need to add
	// any children to the answer.
	if os_path.Equal(delegate_node.path) {
		for _, child_node := range delegate_node.children {
			if utils.InString(names, child_node.name) {
				continue
			}

			// The child node represents a new filesystem mounted at the
			// current point in the mount tree. We need to request it to
			// do a stat of the prefix within its own namespace.
			child_stat, err := child_node.accessor.LstatWithOSPath(
				child_node.prefix)
			if err == nil {
				child_name := child_stat.Name()
				names = append(names, child_name)
				res = append(res, &FileInfoWrapper{
					FileInfo:      child_stat,
					prefix:        child_node.path.Copy(),
					remove_prefix: child_node.prefix.Copy(),
				})
			}
		}
	}

	return res, nil
}

func (self *MountFileSystemAccessor) getDelegatePath(path *OSPath) (
	*node, *OSPath, error) {
	delegate_node, residual, err := self.getDelegateNode(path)
	if err != nil {
		return nil, nil, err
	}
	deep_delegate_path := delegate_node.prefix.Append(residual...)
	return delegate_node, deep_delegate_path, nil
}

func (self *MountFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	// Parse the path into an OSPath
	os_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(os_path)
}

func (self *MountFileSystemAccessor) OpenWithOSPath(
	os_path *OSPath) (ReadSeekCloser, error) {
	delegate_node, delegate_path, err := self.getDelegatePath(os_path)
	if err != nil {
		return nil, err
	}
	return delegate_node.accessor.OpenWithOSPath(delegate_path)
}

func (self *MountFileSystemAccessor) Lstat(path string) (FileInfo, error) {
	// Parse the path into an OSPath
	os_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(os_path)
}

func (self *MountFileSystemAccessor) LstatWithOSPath(os_path *OSPath) (FileInfo, error) {
	delegate_node, delegate_path, err := self.getDelegatePath(os_path)
	if err != nil {
		return nil, err
	}
	file_info, err := delegate_node.accessor.LstatWithOSPath(delegate_path)
	if err != nil {
		return nil, err
	}

	// Wrap the file info before returning it.
	return &FileInfoWrapper{
		FileInfo:      file_info,
		prefix:        delegate_node.path.Copy(),
		remove_prefix: delegate_node.prefix.Copy(),
	}, nil
}

// Install a mapping from the source to the target. This means that
// operating on paths below the target will act on the
// source. Examples:
//
// source = /mnt/bin, target = /bin, accessor = file
// means Open(/bin/foo) redirects to /mnt/bin/foo with accessor "file".

func (self *MountFileSystemAccessor) AddMapping(
	source *OSPath,
	target *OSPath,
	source_accessor FileSystemAccessor) {

	// Walk the tree and create the sentinal node. NOTE: split the
	// path according to the target accessor we are emulating.
	node := self.root

	for _, c := range target.Components {
		if c != "" {
			node = node.MakeChild(c)
		}
	}

	// Install the node in the tree - this is where we read from.
	node.prefix = source.Copy()
	node.accessor = source_accessor
	node.last_mount_point = node
}

func NewMountFileSystemAccessor(
	root_path *OSPath, root FileSystemAccessor) *MountFileSystemAccessor {

	result := &MountFileSystemAccessor{
		root: &node{
			accessor: root,
			path:     root_path.Copy(),
			prefix:   root_path.Copy(),
		},
	}
	result.root.last_mount_point = result.root

	return result
}

func init() {
	json.RegisterCustomEncoder(&FileInfoWrapper{}, MarshalGlobFileInfo)
}
