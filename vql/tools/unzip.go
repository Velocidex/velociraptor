//go:build extras
// +build extras

package tools

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
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
	Filename        *accessors.OSPath `vfilter:"required,field=filename,doc=File to unzip."`
	Accessor        string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	FilenameFilter  string            `vfilter:"optional,field=filename_filter,doc=Only extract members matching this regex filter."`
	OutputDirectory string            `vfilter:"required,field=output_directory,doc=Where to unzip to"`
	Type            string            `vfilter:"optional,field=type,doc=The type of file (default autodetected from file extension - zip or tgz or tar.gz)."`
}

type UnzipPlugin struct{}

func (self UnzipPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "unzip", args)()

		err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
		if err != nil {
			scope.Log("unzip: %s", err)
			return
		}

		arg := &UnzipPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("unzip: %s", err.Error())
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

		// Make sure we are allowed to write there.
		err = file.CheckPath(arg.OutputDirectory)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		output_directory, err := filepath.Abs(arg.OutputDirectory)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		s, err := accessor.LstatWithOSPath(arg.Filename)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		fd, err := accessor.OpenWithOSPath(arg.Filename)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		if arg.Type == "" {
			// Try to guess the type from the file extension.
			base := strings.ToLower(arg.Filename.Basename())
			if strings.HasSuffix(base, ".tgz") ||
				strings.HasSuffix(base, ".tar.gz") {
				arg.Type = "tgz"
			} else if strings.HasSuffix(base, ".zip") {
				arg.Type = "zip"
			}
		}

		switch arg.Type {
		case "tgz":
			err = self.unpackTGZ(ctx, scope, fd, s.Size(),
				filter_reg, output_directory, output_chan)
			if err != nil {
				scope.Log("unzip: %v", err)
			}

		case "zip":
			err = self.unpackZip(ctx, scope, fd, s.Size(),
				filter_reg, output_directory, output_chan)
			if err != nil {
				scope.Log("unzip: %v", err)
			}

		default:
			scope.Log("unzip: unknown file type %v", arg.Type)
		}
	}()
	return output_chan
}

func (self *UnzipPlugin) unpackZip(
	ctx context.Context,
	scope vfilter.Scope,
	fd io.ReadSeeker, size int64,
	filter_reg *regexp.Regexp,
	output_directory string,
	output_chan chan vfilter.Row) error {

	zip, err := zip.NewReader(utils.MakeReaderAtter(fd), size)
	if err != nil {
		return err
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

		err = os.MkdirAll(filepath.Dir(output_path), 0700)
		if err != nil {
			return err
		}

		out_fd, err := os.OpenFile(output_path,
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if err != nil {
			return err
		}
		defer out_fd.Close()

		in_fd, err := member.Open()
		if err != nil {
			return err
		}

		n, err := utils.Copy(ctx, out_fd, in_fd)
		if err != nil {
			in_fd.Close()
			return err
		}
		in_fd.Close()

		output := &UnzipResponse{
			OriginalPath: member.Name,
			NewPath:      output_path,
			Size:         int64(n),
		}

		select {
		case <-ctx.Done():
			return nil
		case output_chan <- output:
		}
	}
	return nil
}

func (self *UnzipPlugin) unpackTGZ(
	ctx context.Context,
	scope vfilter.Scope,
	fd io.Reader, size int64,
	filter_reg *regexp.Regexp,
	output_directory string,
	output_chan chan vfilter.Row) error {

	gzr, err := gzip.NewReader(fd)
	if err == nil {
		defer gzr.Close()
		fd = gzr
	}

	tr := tar.NewReader(fd)

	for {
		member, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil

		case err != nil:
			return err

		case member == nil:
			continue
		}

		if !filter_reg.MatchString(member.Name) ||
			member.Typeflag != tar.TypeReg {
			continue
		}

		// Sanitize the name for writing.
		output_path := filepath.Join(output_directory, member.Name)

		// Directory traversal ...
		if !strings.HasPrefix(output_path, output_directory) {
			continue
		}

		err = os.MkdirAll(filepath.Dir(output_path), 0700)
		if err != nil {
			return err
		}

		out_fd, err := os.OpenFile(output_path,
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if err != nil {
			return err
		}
		defer out_fd.Close()

		n, err := utils.Copy(ctx, out_fd, tr)
		if err != nil {
			return err
		}

		output := &UnzipResponse{
			OriginalPath: member.Name,
			NewPath:      output_path,
			Size:         int64(n),
		}

		select {
		case <-ctx.Done():
			return nil

		case output_chan <- output:
		}
	}
}

func (self UnzipPlugin) Name() string {
	return "unzip"
}

func (self UnzipPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "unzip",
		Doc:      "Unzips a file into a directory",
		ArgType:  type_map.AddType(scope, &UnzipPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE, acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UnzipPlugin{})
}
