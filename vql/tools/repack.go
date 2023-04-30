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
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
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
	embedded_re     = regexp.MustCompile(`#{3}<Begin Embedded Config>\r?\n`)
	embedded_msi_re = regexp.MustCompile(`## Velociraptor client configuration`)
)

const (
	MAX_MEMORY = 200 * 1024 * 1024
	MSI_MAGIC  = "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"
)

type RepackFunctionArgs struct {
	Target     string            `vfilter:"optional,field=target,doc=The name of the target OS to repack (VelociraptorWindows, VelociraptorLinux, VelociraptorDarwin)"`
	Version    string            `vfilter:"optional,field=version,doc=Velociraptor Version to repack"`
	Exe        *accessors.OSPath `vfilter:"optional,field=exe,doc=Alternative a path to the executable to repack"`
	Accessor   string            `vfilter:"optional,field=accessor,doc=The accessor to use to read the file."`
	Binaries   []string          `vfilter:"optional,field=binaries,doc=List of tool names that will be repacked into the target"`
	Config     string            `vfilter:"required,field=config,doc=The config to be repacked in the form of a json or yaml string"`
	UploadName string            `vfilter:"required,field=upload_name,doc=The name of the upload to create"`
}

type RepackFunction struct{}

func (self RepackFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

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

	exe_bytes, err := readExeFile(ctx, config_obj, scope,
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

	if b.Len() > len(config.FileConfigDefaultYaml)-40 {
		return fmt.Errorf("config file is too large to embed.")
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

func readExeFile(
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

		return ioutil.ReadAll(fd)
	}

	// Fetch the tool definition
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

	match := embedded_msi_re.FindIndex(data)
	if match == nil || match[1] < 10 {
		scope.Log("client_repack: I can not seem to locate the embedded config???? To repack an MSI, be sure to build from custom.xml with the custom.config.yaml file.")
		return vfilter.Null{}
	}

	end := match[0]

	// null out the checksum because we are too lazy to calculate it.
	data[end-8] = 0
	data[end-7] = 0
	data[end-6] = 0
	data[end-5] = 0

	// Copy the config data on top of the old one
	for i := 0; i < len(config_data); i++ {
		data[end+i] = config_data[i]
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

		n, err := utils.Copy(ctx, outfd, fd)
		fd.Close()

		csv_writer.Write(ordereddict.NewDict().
			Set("ToolName", tool.Name).
			Set("Filename", "uploads/"+tool.Filename).
			Set("ExpectedHash", tool.Hash).
			Set("Size", n))
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
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RepackFunction{})
}
