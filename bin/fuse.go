// +build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	fuse_command = app.Command(
		"fuse", "Mount a client as a fuse mount.")
	fuse_command_mnt_point = fuse_command.Arg(
		"mnt_point", "Mount point.").
		Required().String()
	fuse_command_client = fuse_command.Arg(
		"client_id", "Client ID to mount.").
		Required().String()
)

type VFSFs struct {
	pathfs.FileSystem
	config_obj *config_proto.Config
	client_id  string

	// Cache directory listings.
	cache  map[string][]*api.FileInfoRow
	logger *logging.LogContext
}

func (self *VFSFs) fetchDir(vfs_name string) ([]*api.FileInfoRow, error) {
	self.logger.Info(fmt.Sprintf("Fetching dir %v from %v", vfs_name, self.client_id))
	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	response, err := client.VFSRefreshDirectory(context.Background(),
		&api_proto.VFSRefreshDirectoryRequest{
			ClientId: self.client_id,
			VfsPath:  vfs_name,
		})
	if err != nil {
		return nil, err
	}

	// Spin here until the flow is complete.
	get_flow_request := &api_proto.ApiFlowRequest{
		ClientId: self.client_id,
		FlowId:   response.FlowId,
	}

	for {
		response, err := client.GetFlowDetails(context.Background(),
			get_flow_request)
		if err != nil {
			return nil, err
		}

		if response.Context.State != flows_proto.FlowContext_RUNNING {
			break
		}

		time.Sleep(200 * time.Millisecond)
	}

	return self.getDir(vfs_name)
}

func (self *VFSFs) fetchFile(vfs_name string) error {
	self.logger.Info("Fetching file %v", vfs_name)

	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	flow_runner_args := &flows_proto.FlowRunnerArgs{
		ClientId: self.client_id,
		FlowName: "VFSDownloadFile",
	}

	flow_args, err := ptypes.MarshalAny(&flows_proto.VFSDownloadFileRequest{
		ClientId: self.client_id,
		VfsPath:  []string{"/" + vfs_name},
	})

	if err != nil {
		return err
	}
	flow_runner_args.Args = flow_args

	response, err := client.LaunchFlow(context.Background(), flow_runner_args)
	if err != nil {
		return err
	}

	// Spin here until the flow is complete.
	get_flow_request := &api_proto.ApiFlowRequest{
		ClientId: self.client_id,
		FlowId:   response.FlowId,
	}

	for {
		response, err := client.GetFlowDetails(context.Background(),
			get_flow_request)
		if err != nil {
			return err
		}

		if response.Context.State != flows_proto.FlowContext_RUNNING {
			// If there were no files uploaded we could
			// not find the file on the client.
			if response.Context.TotalUploadedFiles == 0 {
				return &os.PathError{Path: vfs_name}
			}
			break
		}

		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

func (self *VFSFs) GetAttr(name string, fcontext *fuse.Context) (*fuse.Attr, fuse.Status) {
	if name == "" {
		return &fuse.Attr{
			Mode: fuse.S_IFDIR | 0755,
		}, fuse.OK
	}

	vfs_name := fsPathToVFS(name)

	dirname, basename := path.Split(vfs_name)
	rows, err := self.getDir(dirname)
	if err != nil {
		rows, err = self.fetchDir(dirname)
		if err != nil {
			self.logger.Error(
				fmt.Sprintf("Failed to fetch %s: %v", dirname, err))
			return nil, fuse.ENOENT
		}
	}

	for _, i := range rows {
		if i.Name == basename {
			mode := fuse.S_IFREG | 0644
			if strings.HasPrefix(i.Mode, "d") ||
				strings.HasPrefix(i.Mode, "L") {
				mode = fuse.S_IFDIR | 0644
			}
			return &fuse.Attr{
				Mode:  uint32(mode),
				Size:  uint64(i.Size),
				Atime: uint64(i.Atime.Unix()),
				Mtime: uint64(i.Mtime.Unix()),
				Ctime: uint64(i.Ctime.Unix()),
			}, fuse.OK
		}
	}

	return nil, fuse.ENOENT
}

func (self *VFSFs) getDir(vfs_name string) ([]*api.FileInfoRow, error) {
	rows, pres := self.cache[vfs_name]
	if pres {
		return rows, nil
	}

	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	request := &flows_proto.VFSListRequest{
		ClientId: self.client_id,
		VfsPath:  vfs_name,
	}

	client := api_proto.NewAPIClient(channel)
	response, err := client.VFSListDirectory(context.Background(), request)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(response.Response), &rows)
	if err != nil {
		return nil, err
	}

	self.cache[vfs_name] = rows

	return rows, nil
}

func (self *VFSFs) OpenDir(fs_name string, fcontext *fuse.Context) (
	[]fuse.DirEntry, fuse.Status) {
	vfs_name := fsPathToVFS(fs_name)
	rows, err := self.getDir(vfs_name)
	if err != nil {
		self.logger.Warn(fmt.Sprintf("Fetching directory %s", vfs_name))
		rows, err = self.fetchDir(vfs_name)
		if err != nil {
			return nil, fuse.ENOENT
		}
	}

	result := []fuse.DirEntry{}
	for _, i := range rows {
		mode := fuse.S_IFREG
		if strings.HasPrefix(i.Mode, "d") ||
			strings.HasPrefix(i.Mode, "L") {
			mode = fuse.S_IFDIR
		}

		result = append(result, fuse.DirEntry{
			Name: vfsPathToFS(i.Name),
			Mode: uint32(mode),
		})
	}

	return result, fuse.OK
}

func (self *VFSFs) Open(fs_name string, flags uint32, fcontext *fuse.Context) (
	nodefs.File, fuse.Status) {

	vfs_name := fsPathToVFS(fs_name)

	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	_, err := client.VFSGetBuffer(context.Background(),
		&api_proto.VFSFileBuffer{
			ClientId: self.client_id,
			VfsPath:  vfs_name,
		})
	if err != nil {
		err := self.fetchFile(vfs_name)
		if err != nil {
			_, ok := errors.Cause(err).(*os.PathError)
			if ok {
				return nil, fuse.ENOENT
			}

			return nil, fuse.EIO
		}
	}

	// Get the attributes
	attr, ferr := self.GetAttr(fs_name, fcontext)
	if ferr != fuse.OK {
		return nil, ferr
	}

	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}
	return nodefs.NewReadOnlyFile(&VFSFileReader{
		File:       nodefs.NewDefaultFile(),
		client_id:  self.client_id,
		VfsPath:    vfs_name,
		config_obj: self.config_obj,
		attr:       attr,
		logger:     self.logger,
	}), fuse.OK
}

type VFSFileReader struct {
	nodefs.File
	client_id  string
	VfsPath    string
	attr       *fuse.Attr
	config_obj *config_proto.Config
	logger     *logging.LogContext
}

func (self *VFSFileReader) GetAttr(out *fuse.Attr) fuse.Status {
	*out = *self.attr
	return fuse.OK
}

func (self *VFSFileReader) Read(dest []byte, off int64) (
	fuse.ReadResult, fuse.Status) {

	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	response, err := client.VFSGetBuffer(context.Background(),
		&api_proto.VFSFileBuffer{
			ClientId: self.client_id,
			VfsPath:  self.VfsPath,
			Offset:   uint64(off),
			Length:   uint32(len(dest)),
		})
	if err != nil {
		self.logger.Error("VFSFileReader ", self.VfsPath, err)
		return nil, fuse.ENOENT
	}

	return fuse.ReadResultData(response.Data), fuse.OK
}

func NewVFSFs(config_obj *config_proto.Config, client_id string) *VFSFs {
	return &VFSFs{
		FileSystem: pathfs.NewDefaultFileSystem(),
		client_id:  client_id,
		config_obj: config_obj,
		cache:      make(map[string][]*api.FileInfoRow),
		logger:     logging.GetLogger(config_obj, &logging.ToolComponent),
	}
}

// The VFS path can represent any character in the filename and has
// the path separated by "/", while the filesystem must be much more
// conservative about its representation. Therefore we need to convert
// from the fs name to a VFS name and back again.
func vfsPathToFS(vfs_path string) string {
	components := []string{}
	for _, component := range strings.Split(vfs_path, "/") {
		components = append(components,
			string(datastore.SanitizeString(component)))
	}

	return filepath.Join(components...)
}

func fsPathToVFS(fs_path string) string {
	components := []string{}
	for _, component := range strings.Split(fs_path, string(os.PathSeparator)) {
		components = append(components,
			string(datastore.UnsanitizeComponent(component)))
	}

	return path.Join(components...)
}

func doFuse() {
	config_obj := get_config_or_default()

	vfs_fs := NewVFSFs(config_obj, *fuse_command_client)
	nfs := pathfs.NewPathNodeFs(vfs_fs, nil)
	server, _, err := nodefs.MountRoot(*fuse_command_mnt_point, nfs.Root(), nil)
	if err != nil {
		kingpin.Fatalf("Mount fail: %v\n", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		// Wait for the signal on this channel.
		<-quit
		vfs_fs.logger.Info("Unmounting due to interrupt.")
		for {
			err := server.Unmount()
			if err == nil {
				break
			}

			vfs_fs.logger.Info(fmt.Sprintf("Unable to unmount: %v.", err))
			time.Sleep(1 * time.Second)
		}
	}()

	vfs_fs.logger.Info(fmt.Sprintf(
		"Mounting FUSE filesystem on %v for client %v.", *fuse_command_mnt_point,
		*fuse_command_client))

	defer server.Unmount()

	server.Serve()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case fuse_command.FullCommand():
			doFuse()
		default:
			return false
		}
		return true
	})
}
