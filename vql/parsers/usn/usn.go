package usn

import (
	"context"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/ntfs/readers"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type USNPluginArgs struct {
	Device   *accessors.OSPath `vfilter:"required,field=device,doc=The device file to open."`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	StartUSN int64             `vfilter:"optional,field=start_offset,doc=The starting offset of the first USN record to parse."`
}

type USNPlugin struct{}

func (self USNPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &USNPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_usn: %v", err)
			return
		}

		// If an accessor is not specified we interpret the path as an
		// NTFS path.
		if arg.Accessor == "" {
			arg.Accessor = "ntfs"
			arg.Device, err = accessors.NewWindowsNTFSPath(arg.Device.String())
			if err != nil {
				scope.Log("parse_usn: %v", err)
				return
			}
		}

		device, accessor, err := readers.GetRawDeviceAndAccessor(
			scope, arg.Device, arg.Accessor)
		if err != nil {
			scope.Log("parse_usn: %v", err)
			return
		}

		ntfs_ctx, err := readers.GetNTFSContext(scope, device, accessor)
		if err != nil {
			scope.Log("parse_usn: %v", err)
			return
		}
		defer ntfs_ctx.Close()

		options := readers.GetScopeOptions(scope)
		options.PrefixComponents = arg.Device.Components
		ntfs_ctx.SetOptions(options)

		for item := range ntfs.ParseUSN(ctx, ntfs_ctx, arg.StartUSN) {
			output_chan <- makeUSNRecord(item)
		}
	}()

	return output_chan
}

func (self USNPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_usn",
		Doc:     "Parse the USN journal from a device.",
		ArgType: type_map.AddType(scope, &USNPluginArgs{}),
	}
}

type WatchUSNPluginArgs struct {
	Device string `vfilter:"required,field=device,doc=The device file to open (as an NTFS device)."`
}

type WatchUSNPlugin struct{}

func (self WatchUSNPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &WatchUSNPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_usn: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("watch_usn: %v", err)
			return
		}

		event_channel := make(chan vfilter.Row)

		// We need to interpret the device as an NTFS path in all
		// cases because we can only really watch a raw NTFS partition
		// (it does not make sense to watch a static file).
		ntfs_device, err := accessors.NewWindowsNTFSPath(arg.Device)
		if err != nil {
			scope.Log("watch_usn: %v", err)
			return
		}

		// Register our interest in the log.
		cancel, err := GlobalEventLogService.Register(
			ntfs_device, "ntfs", ctx, config_obj, scope, event_channel)
		if err != nil {
			scope.Log("watch_usn: %v", err)
			return
		}

		defer cancel()

		for {
			select {
			case <-ctx.Done():
				scope.Log("Finished watch_usn() on drive %v", arg.Device)
				return

			case event, ok := <-event_channel:
				if !ok {
					return
				}
				output_chan <- event
			}
		}
	}()

	return output_chan
}

func (self WatchUSNPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "watch_usn",
		Doc:     "Watch the USN journal from a device.",
		ArgType: type_map.AddType(scope, &WatchUSNPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&WatchUSNPlugin{})
	vql_subsystem.RegisterPlugin(&USNPlugin{})
}

func makeUSNRecord(item *ntfs.USN_RECORD) *ordereddict.Dict {
	links := item.Links()
	fullpath := ""
	if len(links) > 0 {
		fullpath = links[0]
	}

	return ordereddict.NewDict().
		Set("Usn", item.Usn()).
		Set("Timestamp", item.TimeStamp().Time).
		Set("Filename", item.Filename()).
		Set("_Links", links).
		Set("FullPath", fullpath).
		Set("FileAttributes", item.FileAttributes()).
		Set("Reason", item.Reason()).
		Set("SourceInfo", item.SourceInfo()).
		Set("_FileMFTID", item.FileReferenceNumberID()).
		Set("_FileMFTSequence", item.FileReferenceNumberSequence()).
		Set("_ParentMFTID", item.ParentFileReferenceNumberID()).
		Set("_ParentMFTSequence", item.ParentFileReferenceNumberSequence())
}
