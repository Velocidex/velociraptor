// +build windows

/*
A fuse implementation using cgofuse and WinFSp.

In order for this to work you must install WinFSp from:

https://github.com/billziss-gh/winfsp/releases/download/v1.4.19049/winfsp-1.4.19049.msi

This is based on a fork from https://github.com/billziss-gh/cgofuse

It would be nice to use that for the linux Fuse as well, but it links
with libfuse on linux to create a non-shared library so for now we use
the old fuse bindings on linux and this one for windows.

*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/cgofuse/fuse"
	"github.com/golang/protobuf/ptypes"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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
		"client_id", "Client ID to mount.").Required().String()

	fuse_args = fuse_command.Flag(
		"fuse_args", "Extra args to pass to the fuse layer.").Strings()

	trace_print = false
)

func trace_debug(msg string, vals ...interface{}) {
	r := recover()
	if r != nil {
		fmt.Printf("PANIC %v\n", r)
		debug.PrintStack()

	} else if trace_print {
		fmt.Printf(msg, vals...)
	}
}

type DirCache struct {
	// Cache directory listings.
	mu    sync.Mutex
	cache map[string][]*api.FileInfoRow
}

func (self *DirCache) Get(key string) ([]*api.FileInfoRow, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.cache[key]
	return res, pres
}

func (self *DirCache) Set(key string, value []*api.FileInfoRow) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.cache[key] = value
}

type VFSFs struct {
	fuse.FileSystemBase

	config_obj *api_proto.Config
	client_id  string
	logger     *logging.LogContext

	cache *DirCache
}

func (self *VFSFs) Getattr(file_path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	if file_path == "/" {
		stat.Mode = fuse.S_IFDIR | 0755
		return 0
	}
	defer trace_debug("Getattr %v", file_path)

	vfs_name := fsPathToVFS(file_path)

	dirname, basename := path.Split(vfs_name)
	rows, err := self.GetDir(dirname)
	if err < 0 {
		self.logger.Err("Getattr %s: %v", dirname, err)
		return err
	}

	for _, row := range rows {
		if row.Name == basename {
			*stat = *newStatF(row)
			return 0
		}
	}

	// File not found in the list of rows.
	return -fuse.ENOENT
}

func (self *VFSFs) Open(path string, flags int) (errc int, fh uint64) {
	return 0, 0
}

func (self *VFSFs) Read(file_path string, buff []byte, off int64, fd uint64) (n int) {
	defer trace_debug("Read %v @ %v\n", file_path, off)

	vfs_name := fsPathToVFS(file_path)
	ferr := self.read_buffer(vfs_name, buff, off, fd)
	if ferr >= 0 {
		return ferr
	}

	// We need to fetch the file from the client.
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
	flow_runner_args.Args = flow_args

	response, err := client.LaunchFlow(context.Background(), flow_runner_args)
	if err != nil {
		self.logger.Err("Fetching Error %s: %v", vfs_name, err)
		return -fuse.EIO
	}

	// Spin here until the flow is done.
	start := time.Now()
	for time.Now().Before(start.Add(30 * time.Second)) {
		is_complete, ferr := self.isFlowComplete(response.FlowId, vfs_name)
		if ferr < 0 {
			return ferr
		}

		if is_complete {
			return self.read_buffer(vfs_name, buff, off, fd)
		}

		self.logger.Info("Flow for %s still outstanding (%v)", vfs_name, response.FlowId)
		time.Sleep(1000 * time.Millisecond)
	}

	return -fuse.EAGAIN
}

func (self *VFSFs) read_buffer(vfs_name string, buff []byte, off int64, fh uint64) (n int) {
	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	response, err := client.VFSGetBuffer(context.Background(),
		&api_proto.VFSFileBuffer{
			ClientId: self.client_id,
			VfsPath:  vfs_name,
			Offset:   uint64(off),
			Length:   uint32(len(buff)),
		})
	if err != nil {
		self.logger.Error("Read ", vfs_name, err)
		return -fuse.ENOENT
	}

	return copy(buff, response.Data)
}

func (self *VFSFs) Opendir(path string) (errc int, fh uint64) {
	return 0, 0
}

func (self *VFSFs) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {

	defer trace_debug("Readdir %v", path)

	fill(".", nil, 0)
	fill("..", nil, 0)

	vfs_name := fsPathToVFS(path)
	rows, err := self.GetDir(vfs_name)
	if err < 0 {
		self.logger.Error(fmt.Sprintf("Readdir %s: %v", path, err))
		return err
	}

	for _, row := range rows {
		if !fill(vfsPathToFS(row.Name), newStatF(row), 0) {
			break
		}
	}
	return 0
}

func newStatF(row *api.FileInfoRow) *fuse.Stat_t {
	mode := uint32(00644 | fuse.S_IFREG)
	if strings.HasPrefix(row.Mode, "d") {
		mode = uint32(00777 | fuse.S_IFDIR)
	}

	if row.Mtime.Unix() < 0 {
		row.Mtime = time.Unix(0, 0)
	}

	if row.Atime.Unix() < 0 {
		row.Atime = time.Unix(0, 0)
	}

	if row.Ctime.Unix() < 0 {
		row.Ctime = time.Unix(0, 0)
	}

	result := &fuse.Stat_t{
		Mode:     mode,
		Nlink:    1,
		Size:     row.Size,
		Mtim:     fuse.NewTimespec(row.Mtime),
		Atim:     fuse.NewTimespec(row.Atime),
		Ctim:     fuse.NewTimespec(row.Ctime),
		Birthtim: fuse.NewTimespec(row.Ctime),
	}

	return result
}

func (self *VFSFs) GetDir(vfs_name string) ([]*api.FileInfoRow, int) {
	// Check the cache for it.
	rows, pres := self.cache.Get(vfs_name)
	if pres {
		return rows, 0
	}

	rows, err := self.getDir(vfs_name)
	if err == nil {
		self.cache.Set(vfs_name, rows)
		return rows, 0
	}

	// Not there - initiate a new client flow.
	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	response, err := client.VFSRefreshDirectory(context.Background(),
		&api_proto.VFSRefreshDirectoryRequest{
			ClientId: self.client_id,
			VfsPath:  vfs_name,
		})
	if err != nil {
		self.logger.Warn("VFSRefreshDirectoryRequest %s: %v", vfs_name, err)
		return nil, -fuse.ENOENT
	}

	self.logger.Info("Initiating VFSRefreshDirectory for %s (%v)", vfs_name, response.FlowId)

	start := time.Now()
	for time.Now().Before(start.Add(30 * time.Second)) {
		is_complete, err := self.isFlowComplete(response.FlowId, vfs_name)
		if err < 0 {
			return nil, err
		}

		if is_complete {
			rows, err := self.getDir(vfs_name)
			if err == nil {
				self.cache.Set(vfs_name, rows)
				return rows, 0
			}

			break
		}

		self.logger.Info("Flow for %s still outstanding (%v)", vfs_name, response.FlowId)
		time.Sleep(1000 * time.Millisecond)
	}

	// Try again later.
	return nil, -fuse.EAGAIN
}

func (self *VFSFs) isFlowComplete(flow_id, vfs_name string) (bool, int) {
	// Check if the flow is completed yet.
	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	req := &api_proto.ApiFlowRequest{
		ClientId: self.client_id,
		FlowId:   flow_id,
	}
	client := api_proto.NewAPIClient(channel)
	response, err := client.GetFlowDetails(context.Background(), req)
	if err != nil {
		self.logger.Warn("GetFlowDetails %s: %v", vfs_name, err)
		return false, -fuse.ENOENT
	}

	// Not ready yet - try again later.
	if response.Context.State == flows_proto.FlowContext_RUNNING {
		return false, 0
	}
	return true, 0
}

func (self *VFSFs) getDir(vfs_name string) ([]*api.FileInfoRow, error) {
	channel := grpc_client.GetChannel(self.config_obj)
	defer channel.Close()

	request := &flows_proto.VFSListRequest{
		ClientId: self.client_id,
		VfsPath:  vfs_name,
	}

	client := api_proto.NewAPIClient(channel)
	response, err := client.VFSListDirectory(context.Background(), request)
	if err != nil {
		self.logger.Warn("VFSListDirectory error %s (%v)", vfs_name, err)
		return nil, err
	}

	rows := []*api.FileInfoRow{}
	err = json.Unmarshal([]byte(response.Response), &rows)
	if err != nil {
		return nil, err
	}

	return rows, nil
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

func NewVFSFs(config_obj *api_proto.Config, client_id string) *VFSFs {
	self := &VFSFs{
		client_id:  client_id,
		config_obj: config_obj,
		logger:     logging.GetLogger(config_obj, &logging.ToolComponent),
		cache: &DirCache{
			cache: make(map[string][]*api.FileInfoRow),
		},
	}
	return self
}

func doFuse() {
	config_obj := get_config_or_default()

	grpc_client.GetChannel(config_obj)

	args := []string{*fuse_command_mnt_point,
		// Winfsp uses very few threads (2) which may cause a
		// deadlock when the fuse mount is the same system as
		// the client.
		"-oThreadCount=50",
	}

	memfs := NewVFSFs(config_obj, *fuse_command_client)
	host := fuse.NewFileSystemHost(memfs)
	host.SetCapReaddirPlus(true)
	host.Mount("", append(args, *fuse_args...))
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
