/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package functions

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DirnameArgs struct {
	Path     vfilter.Any `vfilter:"required,field=path,doc=Extract directory name of path"`
	Sep      string      `vfilter:"optional,field=sep,doc=Separator to use (default /)"`
	PathType string      `vfilter:"optional,field=path_type,doc=Type of path (e.g. windows, linux)"`
}

type DirnameFunction struct{}

func (self *DirnameFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "dirname", args)()

	arg := &DirnameArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("dirname: %s", err.Error())
		return false
	}

	os_path, err := parsePath(ctx, scope, arg.Path, arg.Sep, arg.PathType)
	if err != nil {
		scope.Log("dirname: %v", err)
		return false
	}

	return os_path.Dirname()
}

func (self DirnameFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "dirname",
		Doc:     "Return the directory path.",
		ArgType: type_map.AddType(scope, &DirnameArgs{}),
	}
}

type BasenameFunction struct{}

func (self *BasenameFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "basename", args)()

	arg := &DirnameArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("basename: %v", err)
		return false
	}

	os_path, err := parsePath(ctx, scope, arg.Path, arg.Sep, arg.PathType)
	if err != nil {
		scope.Log("basename: %v", err)
		return false
	}

	return os_path.Basename()
}

func (self BasenameFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "basename",
		Doc:     "Return the basename of the path.",
		ArgType: type_map.AddType(scope, &DirnameArgs{}),
	}
}

type RelnameFunctionArgs struct {
	Path string `vfilter:"required,field=path,doc=Extract directory name of path"`
	Base string `vfilter:"required,field=base,doc=The base of the path"`
	Sep  string `vfilter:"optional,field=sep,doc=Separator to use (default native)"`
}

type RelnameFunction struct{}

func (self *RelnameFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "relpath", args)()

	arg := &RelnameFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("relpath: %s", err.Error())
		return false
	}

	rel, _ := filepath.Rel(arg.Base, arg.Path)
	if arg.Sep == "/" {
		rel = filepath.ToSlash(rel)
	}

	return rel
}

func (self RelnameFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "relpath",
		Doc:     "Return the relative path of .",
		ArgType: type_map.AddType(scope, &RelnameFunctionArgs{}),
	}
}

type PathJoinArgs struct {
	Components []vfilter.Any `vfilter:"required,field=components,doc=Path components to join."`
	Sep        string        `vfilter:"optional,field=sep,doc=Separator to use (default /)"`
	PathType   string        `vfilter:"optional,field=path_type,doc=Type of path (e.g. 'windows')"`
}

type PathJoinFunction struct{}

func (self *PathJoinFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "path_join", args)()

	arg := &PathJoinArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("path_join: %s", err.Error())
		return false
	}

	components := []string{}

	var os_path *accessors.OSPath

	// Parse each component as a path. This allows callers to provide
	// entire paths to path_join instead of a strict component list.
	for _, c := range arg.Components {
		os_path, err = parsePath(ctx, scope, c, arg.Sep, arg.PathType)
		if err != nil {
			scope.Log("path_join: %v", err)
			return false
		}

		components = append(components, os_path.Components...)
	}

	if os_path == nil {
		os_path, err = parsePath(ctx, scope, "", arg.Sep, arg.PathType)
		if err != nil {
			scope.Log("path_join: %v", err)
			return false
		}
	}

	os_path.Components = components
	return os_path
}

func (self PathJoinFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "path_join",
		Doc:     "Build a path by joining all components.",
		ArgType: type_map.AddType(scope, &PathJoinArgs{}),
	}
}

type PathSplitArgs struct {
	Path     vfilter.Any `vfilter:"required,field=path,doc=Path to split into components."`
	PathType string      `vfilter:"optional,field=path_type,doc=Type of path (e.g. 'windows')"`
}

type PathSplitFunction struct{}

func (self *PathSplitFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "path_split", args)()

	arg := &PathSplitArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("path_split: %s", err.Error())
		return []string{}
	}

	os_path, err := parsePath(ctx, scope, arg.Path, "", arg.PathType)
	if err != nil {
		scope.Log("path_split: %v", err)
		return false
	}

	return os_path.Components
}

func (self PathSplitFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "path_split",
		Doc:     "Split a path into components. Note this is more complex than just split() because it takes into account path escaping.",
		ArgType: type_map.AddType(scope, &PathSplitArgs{}),
	}
}

func parsePath(
	ctx context.Context,
	scope vfilter.Scope,
	path vfilter.Any, sep, path_type string) (
	*accessors.OSPath, error) {

	switch t := path.(type) {
	case vfilter.LazyExpr:
		return parsePath(ctx, scope, t.ReduceWithScope(ctx, scope), sep, path_type)

		// Noop if it is already an OSPath
	case *accessors.OSPath:
		return t, nil

	case string:
		if path_type == "" {
			switch sep {
			case "":
				if runtime.GOOS == "windows" {
					path_type = "windows"
				} else {
					path_type = "generic"
				}

			case "\\":
				path_type = "windows"

			default:
				path_type = "generic"
			}
		}

		return accessors.ParsePath(t, path_type)

	default:
		utils.DlvBreak()
		return nil, fmt.Errorf(
			"Path should be an OSPath or string, not %T", path)
	}
}

func init() {
	vql_subsystem.RegisterFunction(&DirnameFunction{})
	vql_subsystem.RegisterFunction(&BasenameFunction{})
	vql_subsystem.RegisterFunction(&RelnameFunction{})
	vql_subsystem.RegisterFunction(&PathJoinFunction{})
	vql_subsystem.RegisterFunction(&PathSplitFunction{})
}
