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
	"net/url"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UrlArgs struct {
	Scheme   string      `vfilter:"optional,field=scheme,doc=The scheme to use"`
	Host     string      `vfilter:"optional,field=host,doc=The host component"`
	Path     string      `vfilter:"optional,field=path,doc=The path component"`
	Fragment string      `vfilter:"optional,field=fragment,doc=The fragment"`
	Query    vfilter.Any `vfilter:"optional,field=query,doc=A dict representing a query string"`
	Parse    string      `vfilter:"optional,field=parse,doc=A url to parse"`
}

type UrlFunction struct{}

func (self UrlFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "url", args)()

	arg := &UrlArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("url: %s", err.Error())
		return false
	}

	if arg.Parse != "" {
		result, err := url.Parse(arg.Parse)
		if err != nil {
			scope.Log("url: %v", err)
			return false
		}

		return result
	}

	return &url.URL{
		Scheme:   arg.Scheme,
		Host:     arg.Host,
		Path:     normalize_path(arg.Path),
		RawQuery: EncodeParams(arg.Query, scope).Encode(),
		Fragment: arg.Fragment,
	}

}

func EncodeParams(param vfilter.Any, scope vfilter.Scope) url.Values {
	data := url.Values{}
	if param != nil {
		for _, member := range scope.GetMembers(param) {
			value, pres := scope.Associative(param, member)
			if pres {
				slice := reflect.ValueOf(value)
				if slice.Type().Kind() == reflect.Slice {
					for i := 0; i < slice.Len(); i++ {
						value := slice.Index(i).Interface()
						item, ok := value.(string)
						if ok {
							data.Add(member, item)
						}
					}
					continue
				}
				switch value.(type) {
				case vfilter.Null, *vfilter.Null:
					continue
				default:
					data.Add(member, fmt.Sprintf("%v", value))
				}
			}
		}
	}
	return data
}

func normalize_path(path string) string {
	path = filepath.Clean(path)
	path = strings.Replace(path, "\\", "/", -1)
	path = strings.TrimLeft(path, "/")
	if path == "." {
		return ""
	}

	// When we encode the URL Go needs it to be preceded with a /
	// otherwise it gets it wrong.
	return "/" + path
}

func (self UrlFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "url",
		Doc:     "Construct a URL or parse one.",
		ArgType: type_map.AddType(scope, &UrlArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UrlFunction{})
}
