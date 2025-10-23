/*
  Repacks a client with a provided configuration and upload to the
  server.

  Previously this functionality was completely implemented in VQL on
  the server, but this approach requires enabling functionality such
  as execve(), http_client() etc. On servers which restrict server
  side functionality the pure VQL implementation is not functional, so
  we have refactored this into a specific VQL function which we can
  allow based on ACL permissions.
*/

package tools

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	generic_re      = []byte(`#!/bin/sh`)
	embedded_re     = regexp.MustCompile(`#{3}<Begin Embedded Config>\r?\n`)
	embedded_msi_re = regexp.MustCompile(`## Velociraptor client configuration\.? \((.+?)\)`)

	// Legacy MSI were not packed correctly with the marker file so
	// only the first page is usable.
	embedded_msi_re_legacy = regexp.MustCompile(`## Velociraptor client configuration\.`)
)

const (
	// Repacking uses a lot of memory because currently it is all done
	// in memory.
	MAX_MEMORY   = 200 * 1024 * 1024
	OLE_PAGESIZE = 0x1000
	MSI_MAGIC    = "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"
)

type RepackFunctionArgs struct {
	Target       string            `vfilter:"optional,field=target,doc=The name of the target OS to repack (VelociraptorWindows, VelociraptorLinux, VelociraptorDarwin)"`
	Version      string            `vfilter:"optional,field=version,doc=Velociraptor Version to repack"`
	Exe          *accessors.OSPath `vfilter:"optional,field=exe,doc=Alternative a path to the executable to repack"`
	Accessor     string            `vfilter:"optional,field=accessor,doc=The accessor to use to read the file."`
	Binaries     []string          `vfilter:"optional,field=binaries,doc=List of tool names that will be repacked into the target"`
	Config       string            `vfilter:"required,field=config,doc=The config to be repacked in the form of a json or yaml string"`
	UploadName   string            `vfilter:"optional,field=upload_name,doc=The name of the upload to create"`
	DestFilename string            `vfilter:"optional,field=dest_filename,doc=If an upload name is not provided, the file will be written to this path"`
}

type RepackFunction struct{}

func (self RepackFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "repack", args)()

	arg := &RepackFunctionArgs{}
	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
	if err != nil {
		scope.Log("ERROR:client_repack: %v", err)
		return vfilter.Null{}
	}

	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("ERROR:client_repack: %v", err)
		return vfilter.Null{}
	}

	if arg.DestFilename != "" {
		err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
		if err != nil {
			scope.Log("ERROR:client_repack: %v", err)
			return vfilter.Null{}
		}

		// Make sure we are allowed to write there.
		err = file.CheckPath(arg.DestFilename)
		if err != nil {
			scope.Log("ERROR:client_repack: %v", err)
			return vfilter.Null{}
		}

	}

	if arg.DestFilename != "" && arg.UploadName != "" ||
		arg.DestFilename == "" && arg.UploadName == "" {
		scope.Log("ERROR:client_repack: One of dest_filename or upload_name should be provided")
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("ERROR:client_repack: Command can only run on the server")
		return vfilter.Null{}
	}

	if arg.Config == "" {
		scope.Log("ERROR:client_repack: Config not specified")
		return vfilter.Null{}
	}

	// Validate the config file.
	_, err = new(config.Loader).
		WithLiteralLoader([]byte(arg.Config)).LoadAndValidate()
	if err != nil {
		scope.Log("ERROR:client_repack: Provided config file not valid: %v", err)
		return vfilter.Null{}
	}

	// If arg.Version is not specified we select the latest version
	// available.
	exe_bytes, err := ReadExeFile(ctx, config_obj, scope,
		arg.Exe, arg.Accessor, arg.Target, arg.Version)
	if err != nil {
		scope.Log("ERROR:client_repack: %v", err)
		return vfilter.Null{}
	}

	// Are we repacking an MSI?
	if len(exe_bytes) > 8 && string(exe_bytes[:8]) == MSI_MAGIC {
		return RepackMSI(ctx, scope, arg.UploadName,
			exe_bytes, []byte(arg.Config))
	}

	scope.Log("client_repack: Will Repack the Velociraptor binary with %v bytes of config",
		len(arg.Config))

	// Compress the string.
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err = w.Write([]byte(arg.Config))
	if err != nil {
		scope.Log("ERROR:client_repack: %v", err)
		return vfilter.Null{}
	}
	w.Close()

	exe_bytes, err = resizeEmbeddedSize(exe_bytes, b.Len())
	if err != nil {
		scope.Log("ERROR:client_repack: %v", err)
		return vfilter.Null{}
	}

	compressed_config_data := b.Bytes()

	// If binaries are specified, do them as well.
	if len(arg.Binaries) > 0 {
		exe_bytes, err = AppendBinaries(
			ctx, config_obj, scope, exe_bytes, arg.Binaries)
		if err != nil {
			scope.Log("ERROR:client_repack: %v", err)
			return vfilter.Null{}
		}
	}

	match := embedded_re.FindIndex(exe_bytes)
	if match == nil {
		scope.Log("ERROR:client_repack: I can not seem to locate the embedded config????")
		return vfilter.Null{}
	}

	end := match[1]

	if len(exe_bytes) < end+len(compressed_config_data) {
		scope.Log("ERROR:client_repack: I can not seem to locate the embedded config????")
		return vfilter.Null{}
	}

	for i := 0; i < len(compressed_config_data); i++ {
		exe_bytes[end+i] = compressed_config_data[i]
	}

	// Write to a local file.
	if arg.DestFilename != "" {
		fd, err := os.OpenFile(arg.DestFilename,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			scope.Log("ERROR:client_repack: %v", err)
			return vfilter.Null{}
		}
		defer fd.Close()

		_, err = fd.Write(exe_bytes)
		if err != nil {
			scope.Log("ERROR:client_repack: %v", err)
			return vfilter.Null{}
		}

		return arg.DestFilename
	}

	sub_scope := scope.Copy()
	sub_scope.AppendVars(
		ordereddict.NewDict().Set("PACKED_Binary", exe_bytes))

	upload_func, ok := scope.GetFunction("upload")
	if !ok {
		return vfilter.Null{}
	}

	return upload_func.Call(
		ctx, sub_scope, ordereddict.NewDict().
			Set("file", "PACKED_Binary").
			Set("name", arg.UploadName).
			Set("accessor", "scope"))
}

func ReadExeFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	exe *accessors.OSPath,
	accessor_name string,
	target_tool, version string) ([]byte, error) {

	if exe != nil {
		accessor, err := accessors.GetAccessor(accessor_name, scope)
		if err != nil {
			return nil, err
		}

		fd, err := accessor.OpenWithOSPath(exe)
		if err != nil {
			return nil, err
		}
		defer fd.Close()

		return utils.ReadAllWithLimit(fd, MAX_MEMORY)
	}

	// Fetch the tool definition. NOTE: The definitions are in the
	// Server.Internal.ToolDependencies artifact and will become
	// available as soon as that artifact is compiled.
	inventory, err := services.GetInventory(config_obj)
	if err != nil {
		return nil, err
	}

	tool, err := inventory.GetToolInfo(
		ctx, config_obj, target_tool, version)
	if err != nil {
		return nil, err
	}

	// The path is determined by the org specific inventory manager,
	// but must be opened using the root orgs filestore.
	path_manager := paths.NewInventoryPathManager(config_obj, tool)
	pathspec, file_store_factory, err := path_manager.Path()
	if err != nil {
		return nil, err
	}

	fd, err := file_store_factory.ReadFile(pathspec)
	if err != nil {
		return nil, err
	}

	s, err := fd.Stat()
	if err != nil {
		return nil, err
	}

	if s.Size() > MAX_MEMORY {
		return nil, errors.New("Total binary size exceeded")
	}

	// For now read the whole file into memory so we can repack
	// it. TODO: Can we do this in a more efficient way?
	exe_bytes := make([]byte, s.Size())
	n, err := fd.Read(exe_bytes)
	if err != nil {
		return nil, err
	}

	return exe_bytes[:n], nil
}

func resizeEmbeddedSize(
	exe_bytes []byte, required_size int) ([]byte, error) {
	if len(exe_bytes) < 100 {
		return nil, errors.New("Binary is too small to resize")
	}

	// Are we dealing with the generic collector? It has an unlimited
	// size so we can just increase it to the required size.
	if utils.BytesEqual(exe_bytes[:len(generic_re)], generic_re) {
		resize_bytes := make([]byte, len(exe_bytes)+required_size)
		for i := 0; i < len(exe_bytes); i++ {
			resize_bytes[i] = exe_bytes[i]
		}
		return resize_bytes, nil
	}

	// For real binaries we have limited space determined by the
	// compiled in placeholder.
	if required_size > len(config.FileConfigDefaultYaml)-40 {
		return nil, errors.New("config file is too large to embed.")
	}

	return exe_bytes, nil
}

type file_offset_map struct {
	config_file_offset int64
	msi_file_offset    int64
}

// Older MSIs did not pack the marker file correctly, making only one
// page usable.
// https://github.com/Velocidex/velociraptor/issues/4304
func extractPageIndexLegacy(scope vfilter.Scope, data []byte) (
	page_idx_map map[int64]int64, cab_header_length int64) {
	res := make(map[int64]int64)

	for _, match := range embedded_msi_re_legacy.FindAllSubmatchIndex(data, -1) {
		if len(match) < 2 {
			continue
		}

		msi_offset := int64(match[0])
		page_offset := msi_offset - (msi_offset % OLE_PAGESIZE)
		cab_header_length := int64(msi_offset % OLE_PAGESIZE)

		res[0] = page_offset
		return res, cab_header_length
	}

	return res, 0
}

func extractPageIndex(scope vfilter.Scope, data []byte) (
	page_idx_map map[int64]int64, cab_header_length int64) {

	// config_file_offset -> MSI file offset
	var offset_map []file_offset_map
	for _, match := range embedded_msi_re.FindAllSubmatchIndex(data, -1) {
		// Need to match the submatch so we can extract it
		if len(match) < 4 {
			continue
		}

		// Start of placeholder (offset of #)
		msi_offset := match[0]
		file_offset, err := strconv.ParseInt(string(data[match[2]:match[3]]), 0, 64)
		if err != nil {
			scope.Log("client_repack: Unable to parse placeholder offset: %v",
				string(data[match[0]:match[1]]))
			continue
		}

		if file_offset == 0 {
			cab_header_length = int64(msi_offset % OLE_PAGESIZE)
		}

		offset_map = append(offset_map, file_offset_map{
			config_file_offset: file_offset,
			msi_file_offset:    int64(msi_offset),
		})
	}

	res := make(map[int64]int64)

	for _, m := range offset_map {
		// The index of the marker within the config file page - the
		// config file marker starts a bit after the CAB header.
		page_idx := (m.config_file_offset + cab_header_length) / OLE_PAGESIZE

		// The corresponding page index in the MSI file. Pages must be aligned!
		page_offset := m.msi_file_offset - (m.msi_file_offset % OLE_PAGESIZE)

		res[page_idx] = page_offset
	}

	// Can not find any markers - maybe it is a legacy MSI?
	if len(offset_map) == 0 {
		return extractPageIndexLegacy(scope, data)
	}

	return res, cab_header_length
}

func RepackMSI(
	ctx context.Context,
	scope vfilter.Scope, upload_name string,
	data []byte, config_data []byte) vfilter.Any {
	scope.Log("client_repack: Will Repack an MSI file with %v bytes of config",
		len(config_data))

	// Make sure the config_data is at least big enough so we get to
	// the comment section of the placeholder.
	config_data_len := len(config_data)
	for i := 0; i < 2000-config_data_len; i++ {
		config_data = append(config_data, '\n')
	}

	// Ensure the packed config ends with a YAML comment to avoid
	// confusion with the padding being considered part of the yaml.
	config_data = append(config_data, []byte("\n\n\n# Padding")...)

	// The placeholder file is spread across several OLE pages. We
	// need to locate those pages so we can overwrite the correct
	// ones. We do this by locating all the markers in the placeholder
	// file and building a page map.
	page_map, cab_header_length := extractPageIndex(scope, data)
	cab_header_offset, page_0_present := page_map[0]

	if len(page_map) == 0 || cab_header_length == 0 || !page_0_present {
		scope.Log("client_repack: I can not seem to locate the embedded config???? To repack an MSI, be sure to build from the placeholder config as described in https://docs.velociraptor.app/docs/deployment/clients/#building-a-custom-msi-package-from-scratch .")
		return vfilter.Null{}
	}

	// Join the cab header before the config data by making a new
	// slice and copying the old data to it.
	new_config_data := append([]byte{},
		data[cab_header_offset:cab_header_offset+cab_header_length]...)
	new_config_data = append(new_config_data, config_data...)
	config_data = new_config_data

	// null out the checksum because we are too lazy to calculate it.
	config_data[cab_header_length-8] = 0
	config_data[cab_header_length-7] = 0
	config_data[cab_header_length-6] = 0
	config_data[cab_header_length-5] = 0

	// Pad out to page size
	pad := OLE_PAGESIZE - len(config_data)%OLE_PAGESIZE
	for i := 0; i < pad; i++ {
		config_data = append(config_data, ' ')
	}

	// Now copy the config data to the MSI one page at the time.
	for i := int64(0); i < int64(len(config_data))/OLE_PAGESIZE; i++ {
		msi_page_idx, pres := page_map[i]
		if !pres {
			scope.Log("client_repack: Insufficient space reserved in MSI file! Can not locate page %v.", i)
			return vfilter.Null{}
		}

		start := i * OLE_PAGESIZE
		end := start + OLE_PAGESIZE
		if end > int64(len(config_data)) {
			end = int64(len(config_data))
		}
		copy(data[msi_page_idx:msi_page_idx+OLE_PAGESIZE], config_data[start:end])
		scope.Log("DEBUG:client_repack: Copying page %v from %#x to %#x->%#x in MSI\n",
			i, start, msi_page_idx, msi_page_idx+OLE_PAGESIZE)
	}

	sub_scope := scope.Copy()
	sub_scope.AppendVars(
		ordereddict.NewDict().Set("PACKED_MSI", data))

	upload_func, ok := scope.GetFunction("upload")
	if !ok {
		return vfilter.Null{}
	}

	return upload_func.Call(
		ctx, sub_scope, ordereddict.NewDict().
			Set("file", "PACKED_MSI").
			Set("name", upload_name).
			Set("accessor", "scope"))
}

func AppendBinaries(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	exe_bytes []byte, binaries []string) ([]byte, error) {

	// Build the zip file that contains all the binaries.
	csv_file := &bytes.Buffer{}
	csv_writer := csv.GetCSVAppender(
		config_obj, scope, csv_file, true, json.DefaultEncOpts())

	buf := &bytes.Buffer{}
	zip := zip.NewWriter(buf)

	inventory, err := services.GetInventory(config_obj)
	if err != nil {
		return nil, err
	}

	for _, name := range binaries {
		parts := strings.SplitN(name, ":", 2)
		version := ""
		if len(parts) > 1 {
			name = parts[0]
			version = parts[1]
		}

		tool, err := inventory.GetToolInfo(ctx, config_obj, name, version)
		if err != nil {
			return nil, err
		}

		scope.Log("Adding binary %v", tool.Name)

		// Try to open the tool directly from the filestore
		path_manager := paths.NewInventoryPathManager(config_obj, tool)
		pathspec, file_store_factory, err := path_manager.Path()
		if err != nil {
			return nil, err
		}

		fd, err := file_store_factory.ReadFile(pathspec)
		if err != nil {
			return nil, err
		}

		outfd, err := zip.Create("uploads/" + tool.Filename)
		if err != nil {
			fd.Close()
			return nil, err
		}

		n, _ := utils.Copy(ctx, outfd, fd)
		fd.Close()

		csv_writer.Write(ordereddict.NewDict().
			Set("ToolName", tool.Name).
			Set("Filename", "uploads/"+tool.Filename).
			Set("ExpectedHash", tool.Hash).
			Set("Size", n).
			Set("Version", tool.Version))
	}

	csv_writer.Close()
	outfd, err := zip.Create("uploads/inventory.csv")
	if err != nil {
		return nil, err
	}

	_, err = outfd.Write(csv_file.Bytes())
	if err != nil {
		return nil, err
	}

	err = zip.Close()
	if err != nil {
		return nil, err
	}

	return appendPayload(exe_bytes, buf.Bytes()), nil
}

// Appends the payload to the exe adjusting resource headers if
// needed.
func appendPayload(exe_bytes []byte, payload []byte) []byte {
	if payload != nil {
		// A PE file - adjust the size of the .rsrc section to
		// cover the entire binary.
		if string(exe_bytes[0:2]) == "MZ" {
			end_of_file := int64(len(exe_bytes) + len(payload))

			// This is the IMAGE_SECTION_HEADER.Name which
			// is also the start of IMAGE_SECTION_HEADER.
			offset_to_rsrc := bytes.Index(exe_bytes, []byte(".rsrc"))

			// Found it.
			if offset_to_rsrc > 0 {
				// IMAGE_SECTION_HEADER.PointerToRawData is a 32 bit int.
				start_of_rsrc_section := binary.LittleEndian.Uint32(
					exe_bytes[offset_to_rsrc+20:])
				size_of_raw_data := uint32(end_of_file) - start_of_rsrc_section
				binary.LittleEndian.PutUint32(
					exe_bytes[offset_to_rsrc+16:], size_of_raw_data)
			}
		}

		exe_bytes = append(exe_bytes, payload...)
	}

	return exe_bytes
}

func (self RepackFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "repack",
		Doc:      "Repack and upload a repacked binary or MSI to the server.",
		ArgType:  type_map.AddType(scope, &RepackFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER, acls.FILESYSTEM_WRITE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RepackFunction{})
}
