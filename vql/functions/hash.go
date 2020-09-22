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
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"sync"

	"github.com/Velocidex/ordereddict"
	glob "www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	pool = sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, 1024*1024) // 1Mb chunks
			return &buffer
		},
	}
)

type HashResult struct {
	MD5    string
	md5    hash.Hash
	SHA1   string
	sha1   hash.Hash
	SHA256 string
	sha256 hash.Hash
}

type HashFunctionArgs struct {
	Path     string `vfilter:"required,field=path,doc=Path to open and hash."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use"`
}

// The hash fuction calculates a hash of a file. It may be expensive
// so we make it cancelllable.
type HashFunction struct{}

func (self *HashFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &HashFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("hash: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Path == "" {
		return vfilter.Null{}
	}

	cached_buffer := pool.Get().(*[]byte)
	defer pool.Put(cached_buffer)

	buf := *cached_buffer

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("hash: %s", err)
		return vfilter.Null{}
	}

	fs, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("hash: %v", err)
		return vfilter.Null{}
	}

	file, err := fs.Open(arg.Path)
	if err != nil {
		scope.Log("hash %s: %v", arg.Path, err.Error())
		return vfilter.Null{}
	}
	defer file.Close()

	result := HashResult{
		md5:    md5.New(),
		sha1:   sha1.New(),
		sha256: sha256.New(),
	}

	for {
		select {
		case <-ctx.Done():
			return vfilter.Null{}

		default:
			n, err := file.Read(buf)

			// We are done!
			if n == 0 || err == io.EOF {
				if n == 0 {
					result.MD5 = fmt.Sprintf(
						"%x", result.md5.Sum(nil))
					result.SHA1 = fmt.Sprintf(
						"%x", result.sha1.Sum(nil))
					result.SHA256 = fmt.Sprintf(
						"%x", result.sha256.Sum(nil))

					return result
				}

			} else if err != nil {
				scope.Log(err.Error())
				return vfilter.Null{}
			}

			_, _ = result.md5.Write(buf[:n])
			_, _ = result.sha1.Write(buf[:n])
			_, _ = result.sha256.Write(buf[:n])

			vfilter.ChargeOp(scope)
		}
	}
}

func (self HashFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "hash",
		Doc:     "Calculate the hash of a file.",
		ArgType: type_map.AddType(scope, &HashFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&HashFunction{})
}
