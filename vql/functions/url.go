/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type UrlArgs struct {
	Scheme   string `vfilter:"optional,field=scheme,doc=The scheme to use"`
	Host     string `vfilter:"optional,field=host,doc=The host component"`
	Path     string `vfilter:"optional,field=path,doc=The path component"`
	Fragment string `vfilter:"optional,field=fragment,doc=The fragment"`

	Parse string `vfilter:"optional,field=parse,doc=A url to parse"`
}

type UrlFunction struct{}

func (self *UrlFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &UrlArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
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
		Fragment: arg.Fragment,
	}

}

func normalize_path(path string) string {
	path = filepath.Clean(path)
	path = strings.Replace(path, "\\", "/", -1)
	path = strings.TrimLeft(path, "/")
	if path == "." {
		return ""
	}

	// When we encode the URL Go needs it to be preceeded with a /
	// otherwise it gets it wrong.
	return "/" + path
}

func (self UrlFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "url",
		Doc:     "Construct a URL or parse one.",
		ArgType: type_map.AddType(scope, &UrlArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UrlFunction{})
}
