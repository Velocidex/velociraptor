package syslog

import (
	"bufio"
	"compress/gzip"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	DUMP_FILES = true
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
		defer vql_subsystem.RegisterMonitor(ctx, "parse_lines", args)()

		arg := &ScannerPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_lines: %v", err)
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

type fileEntry struct {
	full_path *accessors.OSPath
	cancel    func()
}

type fileManager struct {
	mu    sync.Mutex
	files map[string]*fileEntry

	config_obj    *config_proto.Config
	accessor      string
	ctx           context.Context
	scope         vfilter.Scope
	event_channel chan vfilter.Row
	query         vfilter.StoredQuery
}

func (self *fileManager) AddFiles(
	files []*accessors.OSPath, dump_new bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	new_files := make(map[string]*fileEntry)
	for _, f := range files {
		key := f.String()

		entry, pres := self.files[key]
		if pres {
			new_files[key] = entry
			continue
		}
		entry = &fileEntry{
			full_path: f,
		}

		syslog_service := GlobalSyslogService(self.config_obj)

		entry.cancel = syslog_service.Register(
			f, self.accessor, self.ctx, self.scope,
			self.event_channel)
		new_files[key] = entry

		// This is a new file - dump it if we need to.
		if dump_new {
			accessor, err := accessors.GetAccessor(self.accessor, self.scope)
			if err == nil {
				_ = syslog_service.monitorOnce(
					entry.full_path, self.accessor, accessor, &Cursor{})
			}
		}
	}

	// Delete old trackers if the file is disappeared.
	for key, entry := range self.files {
		_, pres := new_files[key]
		if !pres {
			entry.cancel()
		}
	}

	self.files = new_files

	GlobalSyslogService(self.config_obj).Reap()
}

// Run the query periodically and update the watched files list. NOTE:
// Watched files are never removed, only added by the query.
func (self *fileManager) Start() {
	if self.query == nil {
		return
	}

	sleep_time := 3 * time.Second
	if self.config_obj.Defaults != nil {
		if self.config_obj.Defaults.WatchPluginFrequency > 0 {
			sleep_time = time.Second * time.Duration(
				self.config_obj.Defaults.WatchPluginFrequency)
		}
	}

	// First run through do not dump existing lines
	var files []*accessors.OSPath

	for row := range self.query.Eval(self.ctx, self.scope) {
		full_path_any, pres := self.scope.Associative(row, "OSPath")
		if pres {
			full_path, ok := full_path_any.(*accessors.OSPath)
			if ok {
				files = append(files, full_path)
			}
		}
	}
	self.AddFiles(files, !DUMP_FILES)

	// Now periodically check for updates. If a new file appears by
	// the query, dump it from the start.
	go func() {
		for {
			var files []*accessors.OSPath

			for row := range self.query.Eval(self.ctx, self.scope) {
				full_path_any, pres := self.scope.Associative(row, "OSPath")
				if pres {
					full_path, ok := full_path_any.(*accessors.OSPath)
					if ok {
						files = append(files, full_path)
					}
				}
			}
			self.AddFiles(files, DUMP_FILES)
			utils.SleepWithCtx(self.ctx, sleep_time)
		}
	}()
}

func (self *fileManager) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.files == nil {
		return
	}

	for _, entry := range self.files {
		entry.cancel()
	}

	close(self.event_channel)
	self.files = nil
}

func newFileManager(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	accessor string,
	query vfilter.StoredQuery) *fileManager {
	return &fileManager{
		ctx:           ctx,
		config_obj:    config_obj,
		accessor:      accessor,
		scope:         scope,
		query:         query,
		event_channel: make(chan vfilter.Row),
		files:         make(map[string]*fileEntry),
	}
}

type WatchSyslogPluginArgs struct {
	Filenames  []*accessors.OSPath `vfilter:"optional,field=filename,doc=A list of log files to parse."`
	Accessor   string              `vfilter:"optional,field=accessor,doc=The accessor to use."`
	BufferSize int                 `vfilter:"optional,field=buffer_size,doc=Maximum size of line buffer."`
	Query      vfilter.StoredQuery `vfilter:"optional,field=query,doc=If specified we run this query periodically to watch for new files. Rows must have an OSPath column."`
}

type WatchSyslogPlugin struct{}

func (self WatchSyslogPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "watch_syslog", args)()

		arg := &WatchSyslogPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
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
		manager := newFileManager(ctx, scope, config_obj,
			arg.Accessor, arg.Query)
		defer manager.Close()

		manager.AddFiles(arg.Filenames, !DUMP_FILES)
		manager.Start()

		// Wait until the query is complete.
		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-manager.event_channel:
				if !ok {
					return
				}

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
		ArgType:  type_map.AddType(scope, &WatchSyslogPluginArgs{}),
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
