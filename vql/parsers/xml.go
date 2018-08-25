package parsers

import (
	"context"
	"github.com/clbanning/mxj"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _ParseXMLFunctionArgs struct {
	File     string `vfilter:"required,field=file"`
	Accessor string `vfilter:"optional,field=accessor"`
}
type _ParseXMLFunction struct{}

func (self _ParseXMLFunction) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &_ParseXMLFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_xml: %s", err.Error())
		return vfilter.Null{}
	}

	accessor := glob.GetAccessor(arg.Accessor)
	file, err := accessor.Open(arg.File)
	if err != nil {
		scope.Log("Unable to open file %s", arg.File)
		return &vfilter.Null{}
	}
	defer file.Close()

	mxj.SetAttrPrefix("Attr")
	result, err := mxj.NewMapXmlReader(file)
	if err != nil {
		scope.Log("NewMapXmlReader: %v", err)
		return &vfilter.Null{}
	}

	return result.Old()
}

func (self _ParseXMLFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_xml",
		Doc:     "Parse an XML document into a map.",
		ArgType: type_map.AddType(&_ParseXMLFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_ParseXMLFunction{})
}
