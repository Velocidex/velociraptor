//go:build windows && amd64 && cgo
// +build windows,amd64,cgo

package windows

import (
	"context"
	"os"
	"time"

	"github.com/Velocidex/WinPmem/go-winpmem"
	"github.com/Velocidex/ordereddict"
	winpmem_accessor "www.velocidex.com/golang/velociraptor/accessors/winpmem"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	utils_tempfile "www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/filesystem"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/utils/dict"
)

const (
	DeviceName = `\\.\pmem`
)

type WinpmemArgs struct {
	ServiceName string `vfilter:"optional,field=service,doc=The name of the driver service to install."`
	ImagePath   string `vfilter:"optional,field=image_path,doc=If specified we write a physical memory image on this path."`
	Compression string `vfilter:"optional,field=compression,doc=When writing a memory image use this compression (default none) can be none, s2, snappy, gzip."`
	DriverPath  string `vfilter:"optional,field=driver_path,doc=Specify where to extract the driver - by default we use the temp folder"`
}

type WinpmemFunction struct{}

func (self WinpmemFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "winpmem", args)()

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("winpmem: %s", err)
		return vfilter.Null{}
	}

	arg := &WinpmemArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("winpmem: %s", err.Error())
		return vfilter.Null{}
	}

	// To write the image we need FILESYSTEM_WRITE acl
	if arg.ImagePath != "" {
		err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
		if err != nil {
			scope.Log("winpmem: %s", err)
			return vfilter.Null{}
		}
	}

	if arg.ServiceName == "" {
		arg.ServiceName = "winpmem"
	}

	logger := winpmem_accessor.NewLogger(scope, "winpmem: ")
	imager, err := winpmem.NewImager(DeviceName, logger)
	if err != nil {
		driver, err := winpmem.Winpmem_x64()
		if err != nil {
			scope.Log("winpmem: %v", err)
			return vfilter.Null{}
		}

		var tmpfile *os.File

		if arg.DriverPath == "" {
			// The driver is not installed, lets install the driver to a
			// tempfile.
			tmpfile, err = tempfile.TempFile("*.sys")
			if err != nil {
				scope.Log("winpmem: %v", err)
				return vfilter.Null{}
			}
		} else {
			tmpfile, err = os.OpenFile(arg.DriverPath,
				os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
			if err != nil {
				scope.Log("winpmem: %v", err)
				return vfilter.Null{}
			}
		}

		utils_tempfile.AddTmpFile(tmpfile.Name())

		tmpfile.Write([]byte(driver))
		tmpfile.Close()

		scope.Log("Installing driver from %v", tmpfile.Name())
		err = winpmem.InstallDriver(tmpfile.Name(), arg.ServiceName, logger)
		if err != nil {
			scope.Log("winpmem: %v", err)
			return vfilter.Null{}
		}

		// Driver will only be uninstalled when then root scope is destroyed.
		root_scope := vql_subsystem.GetRootScope(scope)
		root_scope.AddDestructor(func() {
			winpmem.UninstallDriver(tmpfile.Name(), arg.ServiceName, logger)

			// Always try to remove the temp file.
			filesystem.RemoveTmpFile(0, tmpfile.Name(), root_scope)
			utils_tempfile.RemoveTmpFile(tmpfile.Name(), nil)
		})

		imager, err = winpmem.NewImager(DeviceName, logger)
		if err != nil {
			scope.Log("winpmem: %s", err.Error())
			return vfilter.Null{}
		}
	}
	defer imager.Close()

	// We only support this mode now - it is the most reliable.
	imager.SetMode(winpmem.PMEM_MODE_PTE)

	result := dict.RowToDict(ctx, scope, imager.Stats())

	// The user asked for a memory image.
	if arg.ImagePath != "" {
		out_fd, err := os.OpenFile(arg.ImagePath,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
		if err != nil {
			scope.Log("winpmem: %v", err)
			return vfilter.Null{}
		}
		defer out_fd.Close()

		out_fd.Truncate(0)

		start := time.Now()
		defer func() {
			scope.Log("winpmem: Completed imaging in %v", time.Now().Sub(start))
		}()

		compressed_writer, closer, err := winpmem.GetCompressor(
			arg.Compression, out_fd)
		if err != nil {
			scope.Log("winpmem: %v", err)
			return vfilter.Null{}
		}
		defer closer()

		err = imager.WriteTo(ctx, compressed_writer)
		if err != nil {
			scope.Log("winpmem: %v", err)
		}

		out_fd.Close()

		offset, _ := out_fd.Seek(0, os.SEEK_CUR)
		result.Set("ImageSize", offset)
		result.Set("Image", out_fd.Name())
	}

	return result
}

func (self WinpmemFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "winpmem",
		Doc:      "Uses the winpmem driver to take a memory image. This plugin is also needed to facilitate the winpmem accessor.",
		ArgType:  type_map.AddType(scope, &WinpmemArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&WinpmemFunction{})
}
