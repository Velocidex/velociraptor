//go:build !windows
// +build !windows

package fuse

import (
	"context"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

type AccessorFuseFS struct {
	fs.Inode

	config_obj *config_proto.Config

	accessor   accessors.FileSystemAccessor
	containers []*accessors.OSPath

	file_count int

	options *Options
}

func (self *AccessorFuseFS) Close() {
	logger := logging.GetLogger(self.config_obj, &logging.ToolComponent)
	logger.Info("Fuse: Exiting! Dont forget to unmount the filesystem")
}

func (self *AccessorFuseFS) add(
	ctx context.Context,
	accessor accessors.FileSystemAccessor,
	ospath *accessors.OSPath,
	node *fs.Inode) error {

	children, err := accessor.ReadDirWithOSPath(ospath)
	if err != nil {
		return err
	}

	for _, child := range children {
		child_ospath := child.OSPath()
		basename := self.options.RemapPath(child_ospath)
		if child.IsDir() {
			// Check if there is a directory node already
			child_node := node.GetChild(basename)
			if child_node == nil {
				child_node = node.NewPersistentInode(ctx, &fs.Inode{},
					fs.StableAttr{Mode: fuse.S_IFDIR})
				node.AddChild(basename, child_node, true)
			}

			err := self.add(ctx, accessor, child.OSPath(), child_node)
			if err != nil {
				return err
			}

		} else {
			child_node := node.NewPersistentInode(
				ctx, &FileNode{
					accessor: accessor,
					ospath:   child_ospath,
					options:  self.options,
				}, fs.StableAttr{})
			node.AddChild(basename, child_node, true)
			self.file_count++
		}
	}
	return nil
}

// Initialize the filesystem by scanning all the containers.
func (self *AccessorFuseFS) OnAdd(ctx context.Context) {
	logger := logging.GetLogger(self.config_obj, &logging.ToolComponent)

	for _, filename := range self.containers {
		self.options.parseTimestamps(self.accessor, filename)

		start := self.file_count
		err := self.add(ctx, self.accessor, filename, &self.Inode)
		if err != nil {
			logger.Error("Fuse: Unable to load from %v: %v",
				filename.DelegatePath(), err)
		} else {
			logger.Info("Fuse: Loaded %v files from %v",
				self.file_count-start, filename.DelegatePath())
		}
	}

}

func NewAccessorFuseFS(
	ctx context.Context,
	config_obj *config_proto.Config,
	accessor accessors.FileSystemAccessor,
	options *Options,
	files []*accessors.OSPath) (*AccessorFuseFS, error) {

	fs := &AccessorFuseFS{
		containers: files,
		accessor:   accessor,
		config_obj: config_obj,
		options:    options,
	}
	return fs, nil
}

var _ = (fs.NodeOnAdder)((*AccessorFuseFS)(nil))
