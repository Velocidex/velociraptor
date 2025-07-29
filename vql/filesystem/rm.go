package filesystem

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _RmRequest struct {
	Filename string `vfilter:"required,field=filename,doc=Filename to remove."`
}

type _RmFunction struct{}

func (self *_RmFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "rm", args)()

	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("rm: %s", err)
		return false
	}

	arg := &_RmRequest{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("rm: %v", err)
		return false
	}

	// Make sure we are allowed to write there.
	err = file.CheckPath(arg.Filename)
	if err != nil {
		scope.Log("rm: %v", err)
		return false
	}

	// On windows especially we can not remove files that are opened
	// by something else, so we keep trying for a while.
	_, err = os.Stat(arg.Filename)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}

	RemoveFile(ctx, 0, arg.Filename, scope)

	return true
}

// Make sure the file is removed when the query is done.
func RemoveFile(
	ctx context.Context,
	retry int, tmpfile string, scope vfilter.Scope) {
	if retry >= 10 {
		scope.Log("rm: Retry count exceeded - giving up")
		return
	}

	if retry > 0 {
		scope.Log("rm: removing %v (Try %v) IsCtxDone %v",
			tmpfile, retry, utils.IsCtxDone(ctx))
	}

	// On windows especially we can not remove files that are opened
	// by something else, so we keep trying for a while.
	err := os.Remove(tmpfile)
	if err != nil {
		scope.Log("rm: Failed to remove %v: %v, reschedule", tmpfile, err)

		// Add another detructor to try again a bit later.
		err = scope.AddDestructor(func() {
			RemoveFile(ctx, retry+1, tmpfile, scope)
		})
		if err != nil {
			return
		}

	} else {
		if retry > 0 {
			scope.Log("rm: removed %v (After try %v)", tmpfile, retry)
		}
	}
}

func (self _RmFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "rm",
		Doc:      "Remove a file from the filesystem using the API.",
		ArgType:  type_map.AddType(scope, &_RmRequest{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_RmFunction{})
}
