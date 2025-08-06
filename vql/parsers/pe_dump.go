package parsers

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	pe "www.velocidex.com/golang/go-pe"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	utils_tempfile "www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/filesystem"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _PEDumpFunctionArgs struct {
	Pid        uint64 `vfilter:"required,field=pid,doc=The pid to dump."`
	BaseOffset int64  `vfilter:"required,field=base_offset,doc=The offset in the file for the base address."`
	InMemory   uint64 `vfilter:"optional,field=in_memory,doc=By default we store to a tempfile and return the path. If this option is larger than 0, we prepare the file in a memory buffer at the specified limit, to avoid AV alerts on disk access."`
}

type _PEDumpFunction struct{}

func (self _PEDumpFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "pe_dump",
		Doc:      "Dump a PE file from process memory.",
		ArgType:  type_map.AddType(scope, &_PEDumpFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func (self _PEDumpFunction) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer utils.RecoverVQL(scope)
	defer vql_subsystem.RegisterMonitor(ctx, "pe_dump", args)()

	arg := &_PEDumpFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("pe_dump: %v", err)
		return &vfilter.Null{}
	}

	lru_size := vql_subsystem.GetIntFromRow(
		scope, scope, constants.BINARY_CACHE_SIZE)
	if lru_size == 0 {
		lru_size = 100
	}

	accessor, err := accessors.GetAccessor("process", scope)
	if err != nil {
		scope.Log("pe_dump: %s", err)
		return &vfilter.Null{}
	}

	pathspec, err := accessor.ParsePath(fmt.Sprintf("/%d", arg.Pid))
	if err != nil {
		scope.Log("pe_dump: %s", err)
		return &vfilter.Null{}
	}

	fd, err := accessor.OpenWithOSPath(pathspec)
	if err != nil {
		scope.Log("pe_dump: %s", err)
		return &vfilter.Null{}
	}
	defer fd.Close()

	paged_reader, err := ntfs.NewPagedReader(
		utils.MakeReaderAtter(fd), 1024*4, int(lru_size))
	if err != nil {
		scope.Log("pe_dump: %s", err)
		return &vfilter.Null{}
	}

	var reader_size int64 = 1<<62 - 1
	reader := utils.NewOffsetReader(paged_reader, arg.BaseOffset, reader_size)

	pe_file, err := pe.NewPEFileWithSize(reader, reader_size)
	if err != nil {
		return &vfilter.Null{}
	}

	var writer io.WriteSeeker
	var tmpfile *os.File
	var memory_buffer *utils.MemoryBuffer

	if arg.InMemory == 0 {
		tmpfile, err = tempfile.TempFile("tmp*exe")
		if err != nil {
			scope.Log("pe_dump: %v", err)
			return false
		}
		defer tmpfile.Close()

		utils_tempfile.AddTmpFile(tmpfile.Name())

		root_scope := vql_subsystem.GetRootScope(scope)
		_ = root_scope.AddDestructor(func() {
			filesystem.RemoveTmpFile(0, tmpfile.Name(), root_scope)
		})

		writer = tmpfile

	} else {
		memory_buffer = &utils.MemoryBuffer{MaxSize: int(arg.InMemory)}
		writer = memory_buffer
	}

	vm_offset := arg.BaseOffset - int64(pe_file.FileHeader.ImageBase)

	// Copy the PE header to the output
	_, err = writer.Seek(0, os.SEEK_SET)
	if err != nil {
		scope.Log("pe_dump: %v", err)
	}

	_, err = fd.Seek(int64(vm_offset+int64(pe_file.FileHeader.ImageBase)),
		os.SEEK_SET)
	if err != nil {
		scope.Log("pe_dump: %v", err)
	}

	_, err = utils.CopyN(ctx, writer, fd, 0x2000)
	if err != nil {
		scope.Log("pe_dump: %v", err)
	}

	// Copy all the regions to the output
	for _, section := range pe_file.Sections {
		// Validate the section sizes
		if section.Size > 100*1024 {
			section.Size = 100 * 1024
		}

		if section.Size <= 0 {
			continue
		}

		_, err = writer.Seek(section.FileOffset, os.SEEK_SET)
		if err != nil {
			scope.Log("pe_dump: %v", err)
		}

		_, err = fd.Seek(vm_offset+int64(section.VMA), os.SEEK_SET)
		if err != nil {
			scope.Log("pe_dump: %v", err)
		}

		// TODO: Restrict the size to be reasonable.
		_, err = utils.CopyN(ctx, writer, fd, section.Size)
		if err != nil {
			scope.Log("pe_dump: %v", err)
		}
	}

	if arg.InMemory == 0 {
		return tmpfile.Name()
	}
	return memory_buffer.Bytes()
}

func init() {
	vql_subsystem.RegisterFunction(&_PEDumpFunction{})
}
