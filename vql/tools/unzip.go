//+build extras

package tools

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UnzipResponse struct {
	OriginalPath string
	NewPath      string
	Size         int64
}

type UnzipPluginArgs struct {
	Filename        string `vfilter:"required,field=filename,doc=File to unzip."`
	Accessor        string `vfilter:"optional,field=accessor,doc=The accessor to use"`
	FilenameFilter  string `vfilter:"optional,field=filename_filter,doc=Only extract members matching this filter."`
	OutputDirectory string `vfilter:"required,field=output_directory,doc=Where to unzip to"`
}

type UnzipPlugin struct{}

func (self UnzipPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
		if err != nil {
			scope.Log("tempdir: %s", err)
			return
		}

		arg := &UnzipPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("unzip: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("unzip: %s", err)
			return
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		filter := arg.FilenameFilter
		if filter == "" {
			filter = "."
		}

		filter_reg, err := regexp.Compile("(?i)" + filter)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		output_directory, err := filepath.Abs(arg.OutputDirectory)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		s, err := accessor.Lstat(arg.Filename)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		fd, err := accessor.Open(arg.Filename)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		zip, err := zip.NewReader(utils.ReaderAtter{fd}, s.Size())
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		for _, member := range zip.File {
			if !filter_reg.MatchString(member.Name) ||
				// Zip directories end with a /
				strings.HasSuffix(member.Name, "/") {
				continue
			}

			// Sanitize the name for writing.
			output_path := filepath.Join(output_directory, member.Name)

			// Directory traversal ...
			if !strings.HasPrefix(output_path, output_directory) {
				continue
			}

			func() {
				err = os.MkdirAll(filepath.Dir(output_path), 0700)
				if err != nil {
					scope.Log("unzip: %v", err)
					return
				}

				out_fd, err := os.OpenFile(output_path,
					os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
				if err != nil {
					scope.Log("unzip: %v", err)
					return
				}
				defer out_fd.Close()

				in_fd, err := member.Open()
				if err != nil {
					scope.Log("unzip: %v", err)
					return
				}
				defer in_fd.Close()

				n, err := utils.Copy(ctx, out_fd, in_fd)
				if err != nil {
					scope.Log("unzip: %v", err)
					return
				}

				output := &UnzipResponse{
					OriginalPath: member.Name,
					NewPath:      output_path,
					Size:         int64(n),
				}
				output_chan <- output
			}()
		}
	}()
	return output_chan
}

func (self UnzipPlugin) Name() string {
	return "unzip"
}

func (self UnzipPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "unzip",
		Doc:     "Unzips a file into a directory",
		ArgType: type_map.AddType(scope, &UnzipPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UnzipPlugin{})
}
