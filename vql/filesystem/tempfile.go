package filesystem

import (
	"context"
	"io/ioutil"
	"os"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _TempfileRequest struct {
	Data []string `vfilter:"required,field=data"`
}

type _TempfileResponse struct {
	Filename string `vfilter:"required,field=filename"`
}

type TempfileFunction struct{}

func (self *TempfileFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &_TempfileRequest{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("tempfile: %s", err.Error())
		return false
	}

	tmpfile, err := ioutil.TempFile("", "tmp")
	if err != nil {
		scope.Log("tempfile: %s", err.Error())
		return false
	}

	for _, content := range arg.Data {
		_, err := tmpfile.Write([]byte(content))
		if err != nil {
			scope.Log("tempfile: %s", err.Error())
		}
	}

	if err := tmpfile.Close(); err != nil {
		scope.Log("tempfile: %s", err.Error())
		return &vfilter.Null{}
	}

	// Make sure the file is removed when the query is done.
	scope.AddDesctructor(func() {
		scope.Log("tempfile: removing tempfile %v", tmpfile.Name())
		os.Remove(tmpfile.Name())
	})
	return tmpfile.Name()
}

func (self TempfileFunction) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "tempfile",
		Doc:     "Create a temporary file and write some data into it.",
		ArgType: type_map.AddType(scope, &_TempfileRequest{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&TempfileFunction{})
}
