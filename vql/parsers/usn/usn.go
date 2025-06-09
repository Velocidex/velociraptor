package usn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/ntfs/readers"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/uploads"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type USNPluginArgs struct {
	Device        *accessors.OSPath `vfilter:"optional,field=device,doc=The device file to open."`
	ImageFilename *accessors.OSPath `vfilter:"optional,field=image_filename,doc=A raw image to open. You can also provide the accessor if using a raw image file."`
	Accessor      string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	MFTFilename   *accessors.OSPath `vfilter:"optional,field=mft_filename,doc=A path to a raw $MFT file to use for path resolution."`
	USNFilename   *accessors.OSPath `vfilter:"optional,field=usn_filename,doc=A path to a raw USN file to parse. If not provided we extract it from the Device or Image file."`

	StartUSN  int64 `vfilter:"optional,field=start_offset,doc=The starting offset of the first USN record to parse."`
	FastPaths bool  `vfilter:"optional,field=fast_paths,doc=If set we resolve full paths using faster but less accurate algorithm."`

	disable_full_path_resolution bool
}

func (self *USNPluginArgs) GetStreams(scope types.Scope) (
	ntfs_ctx *ntfs.NTFSContext,
	usn_stream ntfs.RangeReaderAt,
	err error,
) {
	// First get the ntfs_ctx from the args provided

	// If we specified the device then we open the streams directly
	// from it.
	ntfs_ctx, source, err := self.getNTFSContext(scope)
	if err != nil {
		// Failing this we add an empty MFT and disable the full name
		// resolution. This helps to resolve some names in the case
		// of just a USN journal $J dump.
		scope.Log("parse_usn: Unable to get MFT, Name resolution will be disabled.")
		// Create an empty ntfs context
		ntfs_ctx = ntfs.GetNTFSContextFromRawMFT(
			bytes.NewReader(nil), 0x200, 0x200)
		err = nil
		self.disable_full_path_resolution = true
	}

	// Now resolve the USN stream.
	if self.USNFilename != nil && len(self.USNFilename.Components) > 0 {
		accessor, err := accessors.GetAccessor(self.Accessor, scope)
		if err != nil {
			return nil, nil, err
		}

		stat, err := accessor.LstatWithOSPath(self.USNFilename)
		if err != nil {
			return nil, nil, fmt.Errorf("%v: %w", self.USNFilename, err)
		}

		reader, err := accessor.OpenWithOSPath(self.USNFilename)
		if err != nil {
			return nil, nil, fmt.Errorf("%v: %w", self.USNFilename, err)
		}

		usn_stream = NewRangeReaderAtWrapper(reader, stat.Size())

	} else {
		scope.Log("Openning default USN stream from %v", source)
		usn_stream, err = ntfs.OpenUSNStream(ntfs_ctx)
		if err != nil {
			return nil, nil, err
		}

	}

	return ntfs_ctx, usn_stream, err
}

func (self *USNPluginArgs) getNTFSContext(scope types.Scope) (
	*ntfs.NTFSContext, *accessors.OSPath, error) {

	if self.Device != nil && len(self.Device.Components) > 0 {
		ntfs_ctx, err := readers.GetNTFSContext(scope, self.Device, "ntfs")
		if err != nil {
			return nil, nil, fmt.Errorf("%v: %w", self.Device, err)
		}
		return ntfs_ctx, self.Device, nil

		// Alternatively we can specify an image file.
	} else if self.ImageFilename != nil &&
		len(self.ImageFilename.Components) > 0 {
		ntfs_ctx, err := readers.GetNTFSContext(
			scope, self.ImageFilename, self.Accessor)
		if err != nil {
			return nil, nil, fmt.Errorf("%v: %w", self.ImageFilename, err)
		}
		return ntfs_ctx, self.ImageFilename, nil

		// We can read an $MFT dump directly.
	} else if self.MFTFilename != nil &&
		len(self.MFTFilename.Components) > 0 {

		ntfs_ctx, err := readers.GetNTFSContextFromRawMFT(
			scope, self.MFTFilename, self.Accessor)
		if err != nil {
			return nil, nil, fmt.Errorf("%v: %w", self.MFTFilename, err)
		}
		return ntfs_ctx, self.MFTFilename, nil
	}

	return nil, nil, errors.New("No MFT specified")
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
		defer vql_subsystem.RegisterMonitor(ctx, "parse_usn", args)()

		arg := &USNPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_usn: %v", err)
			return
		}

		ntfs_ctx, usn_stream, err := arg.GetStreams(scope)
		if err != nil {
			scope.Log("parse_usn: %v", err)
			return
		}
		defer ntfs_ctx.Close()

		options := readers.GetScopeOptions(scope)
		if arg.Device != nil {
			options.PrefixComponents = arg.Device.Components
		}

		if !options.DisableFullPathResolution && arg.disable_full_path_resolution {
			options.DisableFullPathResolution = true
		}
		ntfs_ctx.SetOptions(options)

		if !arg.FastPaths {
			preload_count := 0
			now := utils.GetTime().Now()

			for record := range ntfs.ParseUSN(
				ctx, ntfs_ctx, usn_stream, arg.StartUSN) {
				mft_id := record.FileReferenceNumberID()
				mft_seq := uint16(record.FileReferenceNumberSequence())

				ntfs_ctx.SetPreload(mft_id, mft_seq,
					func(entry *ntfs.MFTEntrySummary) (*ntfs.MFTEntrySummary, bool) {
						if entry != nil {
							return entry, false
						}

						preload_count++

						// Add a fake entry to resolve the filename
						return &ntfs.MFTEntrySummary{
							Sequence: mft_seq,
							Filenames: []ntfs.FNSummary{{
								Name:              record.Filename(),
								NameType:          "DOS+Win32",
								ParentEntryNumber: record.ParentFileReferenceNumberID(),
								ParentSequenceNumber: uint16(
									record.ParentFileReferenceNumberSequence()),
							}},
						}, true
					})
			}

			scope.Log("parse_usn: Preloaded %v USN entries into path resolver in %v",
				preload_count, utils.GetTime().Now().Sub(now))
		}

		for item := range ntfs.ParseUSN(ctx, ntfs_ctx, usn_stream, arg.StartUSN) {
			select {
			case <-ctx.Done():
				return
			case output_chan <- makeUSNRecord(item):
			}
		}
	}()

	return output_chan
}

func (self USNPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_usn",
		Doc:      "Parse the USN journal from a device.",
		ArgType:  type_map.AddType(scope, &USNPluginArgs{}),
		Version:  2,
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
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
		defer vql_subsystem.RegisterMonitor(ctx, "watch_usn", args)()

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
		Set("OSPath", fullpath).
		Set("FileAttributes", item.FileAttributes()).
		Set("Reason", item.Reason()).
		Set("SourceInfo", item.SourceInfo()).
		Set("_FileMFTID", item.FileReferenceNumberID()).
		Set("_FileMFTSequence", item.FileReferenceNumberSequence()).
		Set("_ParentMFTID", item.ParentFileReferenceNumberID()).
		Set("_ParentMFTSequence", item.ParentFileReferenceNumberSequence())
}

type RangeReaderAtWrapper struct {
	io.ReaderAt
	runs []ntfs.Range
}

func (self RangeReaderAtWrapper) Ranges() []ntfs.Range {
	return self.runs
}

func NewRangeReaderAtWrapper(
	reader accessors.ReadSeekCloser, length int64) *RangeReaderAtWrapper {

	result := &RangeReaderAtWrapper{ReaderAt: utils.MakeReaderAtter(reader)}

	range_reader, ok := reader.(uploads.RangeReader)
	if ok {
		for _, run := range range_reader.Ranges() {
			result.runs = append(result.runs, ntfs.Range{
				Offset:   run.Offset,
				Length:   run.Length,
				IsSparse: run.IsSparse,
			})
		}
	} else {
		result.runs = append(result.runs, ntfs.Range{
			Offset:   0,
			Length:   length,
			IsSparse: false,
		})
	}

	return result
}
