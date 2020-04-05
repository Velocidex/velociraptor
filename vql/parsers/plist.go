package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"howett.net/plist"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _PlistParserArgs struct {
	Filename string `vfilter:"required,field=file,doc=A list of files to parse."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type PlistParser struct{}

func (self *PlistParser) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) (result vfilter.Any) {
	arg := &_PlistParserArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("plist: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("plist: %s", err)
		return
	}

	accessor, err := glob.GetAccessor(arg.Accessor, ctx)
	if err != nil {
		scope.Log("pslist: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.Open(arg.Filename)
	if err != nil {
		scope.Log("plist: %v", err)
		return vfilter.Null{}
	}
	defer file.Close()

	var val interface{}
	dec := plist.NewDecoder(file)
	err = dec.Decode(&val)
	if err != nil {
		scope.Log("plist: %v", err)
		return vfilter.Null{}
	}

	return val
}

func (self PlistParser) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "plist",
		Doc:     "Parse plist file",
		ArgType: type_map.AddType(scope, &_PlistParserArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&PlistParser{})
}
