package syslog

import (
	"bufio"
	"compress/gzip"
	"context"
	"io"
	"os"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ScannerPluginArgs struct {
	Filenames  []*accessors.OSPath `vfilter:"required,field=filename,doc=A list of log files to parse."`
	Accessor   string              `vfilter:"optional,field=accessor,doc=The accessor to use."`
	BufferSize int                 `vfilter:"optional,field=buffer_size,doc=Maximum size of line buffer."`
}

type ScannerPlugin struct{}

func (self ScannerPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_lines",
		Doc:      "Parse a file separated into lines.",
		ArgType:  type_map.AddType(scope, &ScannerPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func (self ScannerPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor("parse_lines", args)()

		arg := &ScannerPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_lines: %v", err)
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("parse_lines: %s", err)
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				fd, err := maybeOpenGzip(scope, arg.Accessor, filename)
				if err != nil {
					scope.Log("parse_lines: %v", err)
					return
				}
				defer fd.Close()

				scanner := bufio.NewScanner(fd)

				// Allow the user to increase buffer size (default 64kb)
				if arg.BufferSize > 0 {
					scanner.Buffer(make([]byte, arg.BufferSize), arg.BufferSize)
				}

				firstLine := true
				for scanner.Scan() {
					var line string
					if firstLine {
						// strip UTF-8 byte order mark if any
						line, _ = strings.CutPrefix(scanner.Text(), "\xef\xbb\xbf")
						firstLine = false
					} else {
						line = scanner.Text()
					}

					select {
					case <-ctx.Done():
						return

					case output_chan <- ordereddict.NewDict().
						Set("Line", line):
					}
				}
				err = scanner.Err()
				if err != nil {
					scope.Log("parse_lines: %v", err)
					return
				}
			}()
		}
	}()

	return output_chan
}

type WatchSyslogPlugin struct{}

func (self WatchSyslogPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor("watch_syslog", args)()

		arg := &ScannerPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_syslog: %v", err)
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("watch_syslog: %v", err)
			return
		}

		// This plugin needs to be running on clients which have no
		// server config object.
		client_config_obj, ok := artifacts.GetConfig(scope)
		if !ok {
			scope.Log("watch_syslog: unable to get config")
			return
		}

		config_obj := &config_proto.Config{Client: client_config_obj}

		event_channel := make(chan vfilter.Row)

		// Register the output channel as a listener to the
		// global event.
		for _, filename := range arg.Filenames {
			cancel := GlobalSyslogService(config_obj).Register(
				filename, arg.Accessor, ctx, scope,
				event_channel)

			defer cancel()
		}

		// Wait until the query is complete.
		for {
			select {
			case <-ctx.Done():
				return

			case event := <-event_channel:
				select {
				case <-ctx.Done():
					return

				case output_chan <- event:
				}
			}
		}
	}()

	return output_chan
}

func (self WatchSyslogPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "watch_syslog",
		Doc:      "Watch a syslog file and stream events from it. ",
		ArgType:  type_map.AddType(scope, &ScannerPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func maybeOpenGzip(scope vfilter.Scope,
	accessor_name string,
	filename *accessors.OSPath) (io.ReadCloser, error) {
	accessor, err := accessors.GetAccessor(accessor_name, scope)
	if err != nil {
		return nil, err
	}

	fd, err := accessor.OpenWithOSPath(filename)
	if err != nil {
		return nil, err
	}

	// If the fd is not seekable we can not try to open as a gzip
	// file.
	if !accessors.IsSeekable(fd) {
		return fd, nil
	}

	zr, err := gzip.NewReader(fd)
	if err == nil {
		return zr, nil
	}

	// Rewind the file back if we can
	off, err := fd.Seek(0, os.SEEK_SET)
	if err == nil && off == 0 {
		return fd, nil
	}

	// We can not rewind it - force the file to reopen. This is more
	// expensive but should restore the file to the correct state.
	fd.Close()

	return accessor.OpenWithOSPath(filename)
}

func init() {
	vql_subsystem.RegisterPlugin(&WatchSyslogPlugin{})
	vql_subsystem.RegisterPlugin(&ScannerPlugin{})
}
