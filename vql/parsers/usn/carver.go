package usn

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/ntfs/readers"
	"www.velocidex.com/golang/velociraptor/acls"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type CarveUSNPluginArgs struct {
	Device        *accessors.OSPath `vfilter:"optional,field=device,doc=The device file to open."`
	ImageFilename *accessors.OSPath `vfilter:"optional,field=image_filename,doc=A raw image to open. You can also provide the accessor if using a raw image file."`
	Accessor      string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	MFTFilename   *accessors.OSPath `vfilter:"optional,field=mft_filename,doc=A path to a raw $MFT file to use for path resolution."`
	USNFilename   *accessors.OSPath `vfilter:"optional,field=usn_filename,doc=A path to a raw USN file to carve. If not provided we carve the image file or the device."`

	disable_full_path_resolution bool
}

func (self *CarveUSNPluginArgs) GetStreams(scope types.Scope) (
	ntfs_ctx *ntfs.NTFSContext,
	usn_stream io.ReaderAt,
	size int64,
	err error,
) {

	var mft_source, usn_source *accessors.OSPath

	// First get the ntfs_ctx from the args provided

	// If we specified the device then we open the streams directly
	// from it.
	if self.Device != nil && len(self.Device.Components) > 0 {
		ntfs_ctx, err = readers.GetNTFSContext(scope, self.Device, "ntfs")
		if err != nil {
			return nil, nil, 0, err
		}

		mft_source = self.Device
		usn_source = self.Device

		// Alternatively we can specify an image file.
	} else if self.ImageFilename != nil &&
		len(self.ImageFilename.Components) > 0 {

		accessor, err := accessors.GetAccessor(self.Accessor, scope)
		if err != nil {
			return nil, nil, 0, err
		}

		stat, err := accessor.LstatWithOSPath(self.ImageFilename)
		if err != nil {
			return nil, nil, 0, err
		}

		size = stat.Size()

		ntfs_ctx, err = readers.GetNTFSContext(
			scope, self.ImageFilename, self.Accessor)
		if err != nil {
			return nil, nil, 0, err
		}

		mft_source = self.ImageFilename
		usn_source = self.ImageFilename

		// We can read an $MFT dump directly.
	} else if self.MFTFilename != nil &&
		len(self.MFTFilename.Components) > 0 {

		ntfs_ctx, err = readers.GetNTFSContextFromRawMFT(
			scope, self.MFTFilename, self.Accessor)
		if err != nil {
			return nil, nil, 0, err
		}

		if self.USNFilename == nil || len(self.USNFilename.Components) == 0 {
			return nil, nil, 0,
				errors.New("Must specify usn_filename when an mft_filename is specified.")
		}

		mft_source = self.MFTFilename

		// The USN stream to carve may be given as a separate file.
	} else if self.USNFilename != nil && len(self.USNFilename.Components) > 0 {
		accessor, err := accessors.GetAccessor(self.Accessor, scope)
		if err != nil {
			return nil, nil, 0, err
		}

		stat, err := accessor.LstatWithOSPath(self.USNFilename)
		if err != nil {
			return nil, nil, 0, err
		}

		size = stat.Size()

		usn_stream_fd, err := accessor.OpenWithOSPath(self.USNFilename)
		if err != nil {
			return nil, nil, 0, err
		}

		usn_stream = utils.MakeReaderAtter(usn_stream_fd)
		usn_source = self.USNFilename

		// Otherwise we carve the disk from the ntfs context.

		// Failing this we add an empty MFT - this helps to resolve
		// some names in the case of just a USN journal $J dump.
	} else {
		ntfs_ctx = ntfs.GetNTFSContextFromRawMFT(
			bytes.NewReader(nil), 0x200, 0x200)

		if self.USNFilename == nil || len(self.USNFilename.Components) == 0 {
			return nil, nil, 0,
				errors.New("Must specify usn_filename when not mft source is specified.")
		}

		mft_source = accessors.MustNewGenericOSPath("")
		self.disable_full_path_resolution = true

		usn_stream = ntfs_ctx.DiskReader
		usn_source = mft_source
	}

	if size == 0 && ntfs_ctx.Boot != nil {
		size = ntfs_ctx.Boot.VolumeSize() * int64(ntfs_ctx.Boot.Sector_size())
	}

	scope.Log("carve_usn: Carving %v with size %v (MFT path resolution from %v)",
		usn_source, size, mft_source)

	return ntfs_ctx, usn_stream, size, nil
}

type CarveUSNPlugin struct{}

func (self CarveUSNPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)
		defer vql_subsystem.RegisterMonitor(ctx, "carve_usn", args)()

		arg := &CarveUSNPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("carve_usn: %v", err)
			return
		}

		ntfs_ctx, usn_stream, size, err := arg.GetStreams(scope)
		if err != nil {
			scope.Log("carve_usn: %v", err)
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

		for item := range ntfs.CarveUSN(ctx, ntfs_ctx, usn_stream, size) {
			output_chan <- makeUSNRecord(item.USN_RECORD).Set("DiskOffset", item.DiskOffset)
		}
	}()

	return output_chan
}

func (self CarveUSNPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "carve_usn",
		Doc:      "Carve for the USN journal entries from a device.",
		ArgType:  type_map.AddType(scope, &CarveUSNPluginArgs{}),
		Version:  2,
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&CarveUSNPlugin{})
}
