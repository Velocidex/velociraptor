// +build cgo

package tools

import (
	"context"
	"fmt"

	"github.com/Velocidex/go-magic/magic"
	"github.com/Velocidex/go-magic/magic_files"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	magicHandle = "$MagicHandle"
)

type MagicFunctionArgs struct {
	Path     string `vfilter:"required,field=path,doc=Path to open and hash."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Type     string `vfilter:"optional,field=type,doc=Magic type (can be empty or 'mime' or 'extension')"`
	Magic    string `vfilter:"optional,field=magic,doc=Additional magic to load"`
}

type MagicFunction struct{}

func (self MagicFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &MagicFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("magic: %v", err)
		return vfilter.Null{}
	}

	magic_type := magic.MAGIC_NONE
	switch arg.Type {
	case "mime":
		magic_type = magic.MAGIC_MIME
	case "extension":
		magic_type = magic.MAGIC_EXTENSION
	case "":
		magic_type = magic.MAGIC_NONE
	default:
		scope.Log("magic: unknown type %v", arg.Type)
		return vfilter.Null{}
	}

	var handle *magic.Magic

	// Cache key based on type and custom magic.
	key := fmt.Sprintf("%s_%s_%d", magicHandle, arg.Type, len(arg.Magic))
	cached := vql_subsystem.CacheGet(scope, key)
	switch t := cached.(type) {

	case error:
		return vfilter.Null{}

	case nil:
		handle = magic.NewMagicHandle(magic_type)
		magic_files.LoadDefaultMagic(handle)

		// Do we need to load additional magic tests?
		if arg.Magic != "" {
			handle.LoadBuffer(arg.Magic)
			errors := handle.GetError()
			if errors != "" {
				scope.Log("magic: While loading custom magic: %v", errors)
			}
		}

		// Attach the handle to the root destructor.
		vql_subsystem.GetRootScope(scope).
			AddDestructor(func() { handle.Close() })
		vql_subsystem.CacheSet(scope, key, handle)

	case *magic.Magic:
		handle = t

	default:
		// Unexpected value in cache.
		return vfilter.Null{}
	}

	// Just let libmagic handle the path
	if arg.Accessor == "" {
		return handle.File(arg.Path)
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("magic: %v", err)
		return vfilter.Null{}
	}

	// Read a header from the file and pass to the libmagic
	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("magic: %v", err)
		return vfilter.Null{}
	}

	fd, err := accessor.Open(arg.Path)
	if err != nil {
		return vfilter.Null{}
	}

	buffer := make([]byte, 1024*64)
	_, err = fd.Read(buffer)
	if err != nil {
		return vfilter.Null{}
	}

	return handle.Buffer(buffer)
}

func (self MagicFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "magic",
		Doc:     "Identify a file using magic rules.",
		ArgType: type_map.AddType(scope, &MagicFunctionArgs{}),
		Version: 1,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&MagicFunction{})
}
