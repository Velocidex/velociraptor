package syslog

import (
	"bufio"
	"compress/gzip"
	"context"
	"io"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ScannerPluginArgs struct {
	Filenames []string `vfilter:"required,field=filename,doc=A list of log files to parse."`
	Accessor  string   `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type ScannerPlugin struct{}

func (self ScannerPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_lines",
		Doc:     "Parse a file separated into lines.",
		ArgType: type_map.AddType(scope, &ScannerPluginArgs{}),
	}
}

func (self ScannerPlugin) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ScannerPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
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
				fd, err := maybeOpenGzip(ctx, arg.Accessor, filename)
				if err != nil {
					scope.Log("parse_lines: %v", err)
					return
				}
				defer fd.Close()

				scanner := bufio.NewScanner(fd)
				for scanner.Scan() {
					output_chan <- ordereddict.NewDict().
						Set("Line", scanner.Text())

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

type _WatchSyslogPlugin struct{}

func (self _WatchSyslogPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ScannerPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch_syslog: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("watch_syslog: %s", err)
			return
		}

		// Register the output channel as a listener to the
		// global event.
		for _, filename := range arg.Filenames {
			cancel := GlobalSyslogService.Register(
				filename, arg.Accessor, ctx, scope, output_chan)

			defer cancel()
		}

		// Wait until the query is complete.
		<-ctx.Done()
	}()

	return output_chan
}

func (self _WatchSyslogPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "watch_syslog",
		Doc:     "Watch a syslog file and stream events from it. ",
		ArgType: type_map.AddType(scope, &ScannerPluginArgs{}),
	}
}

func maybeOpenGzip(ctx context.Context, accessor_name, filename string) (io.ReadCloser, error) {
	accessor, err := glob.GetAccessor(accessor_name, ctx)
	if err != nil {
		return nil, err
	}

	fd, err := accessor.Open(filename)
	if err != nil {
		return nil, err
	}

	defer fd.Seek(0, 0)

	zr, err := gzip.NewReader(fd)
	if err == nil {
		return zr, nil
	}

	return fd, nil
}

func init() {
	vql_subsystem.RegisterPlugin(&_WatchSyslogPlugin{})
	vql_subsystem.RegisterPlugin(&ScannerPlugin{})
}
