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
package filesystem

import (
	"context"
	"io"

	"github.com/Velocidex/ahocorasick"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type GrepFunctionArgs struct {
	Path     string   `vfilter:"required,field=path,doc=path to open."`
	Accessor string   `vfilter:"optional,field=accessor,doc=An accessor to use."`
	Keywords []string `vfilter:"required,field=keywords,doc=Keywords to search for."`
	Context  int      `vfilter:"optional,field=context,doc=Extract this many bytes as context around hits."`
}

type GrepFunction struct{}

// The Grep VQL function searches for a literal or regex match inside the file
func (self *GrepFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &GrepFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("grep: %s", err.Error())
		return false
	}

	if arg.Context == 0 {
		arg.Context = 10
	}

	var keywords [][]byte
	for _, item := range arg.Keywords {
		// TODO: Add extra encodings like UTF16
		keywords = append(keywords, []byte(item))
	}

	ah_matcher := ahocorasick.NewMatcher(keywords)
	offset := 0

	buf := make([]byte, 4*1024*1024) // 4Mb chunks

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("grep: %s", err.Error())
		return false
	}

	fs, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log(err.Error())
		return false
	}

	file, err := fs.Open(arg.Path)
	if err != nil {
		scope.Log(err.Error())
		return false
	}
	defer file.Close()

	hits := []*ordereddict.Dict{}

	for {
		select {
		case <-ctx.Done():
			return vfilter.Null{}

		default:
			n, err := file.Read(buf)
			if err == io.EOF {
				return hits

			} else if err != nil {
				scope.Log(err.Error())
				return false
			}

			for _, hit := range ah_matcher.Match(buf[:n]) {
				min_bound := offset + hit - arg.Context
				if min_bound < 0 {
					min_bound = 0
				}

				max_bound := offset + hit + arg.Context
				if max_bound > n {
					max_bound = n
				}

				hits = append(hits, ordereddict.NewDict().
					Set("type", "GrepHit").
					Set("offset", offset+hit).
					Set("min_bound", min_bound).
					Set("max_bound", max_bound).
					Set("context", string(
						buf[min_bound:max_bound])))
			}

			offset += n
			vfilter.ChargeOp(scope)
		}
	}
}

func (self GrepFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "grep",
		Doc:     "Search a file for keywords.",
		ArgType: type_map.AddType(scope, &GrepFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GrepFunction{})
}
