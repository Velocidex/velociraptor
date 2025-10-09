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
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	pool = sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, 64*1024) // 64kb chunks
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

func (self *HashResult) ToDict() *ordereddict.Dict {
	return ordereddict.NewDict().
		Set("MD5", self.MD5).
		Set("SHA1", self.SHA1).
		Set("SHA256", self.SHA256)
}

type HashFunctionArgs struct {
	Path       *accessors.OSPath `vfilter:"required,field=path,doc=Path to open and hash."`
	Accessor   string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	HashSelect []string          `vfilter:"optional,field=hashselect,doc=The hash function to use (MD5,SHA1,SHA256)"`
	MaxSize    uint64            `vfilter:"optional,field=max_size,doc=The maximum size of the file that will be hashed (default 100mb)"`
}

// HashFunction calculates a hash of a file. It may be expensive
// so we make it cancelllable.
type HashFunction struct{}

func (self *HashFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "hash", args)()

	arg := &HashFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("hash: %v", err)
		return vfilter.Null{}
	}

	if arg.MaxSize == 0 {
		arg.MaxSize = vql_subsystem.GetIntFromRow(
			scope, scope, constants.HASH_MAX_SIZE)
	}

	if arg.MaxSize == 0 {
		arg.MaxSize = 100 * 1024 * 1024
	}

	cached_buffer := pool.Get().(*[]byte)
	defer pool.Put(cached_buffer)

	buf := *cached_buffer

	fs, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("hash: %v", err)
		return vfilter.Null{}
	}

	file, err := fs.OpenWithOSPath(arg.Path)
	if err != nil {
		//scope.Log("hash %s: %v", arg.Path.String(), err)
		return vfilter.Null{}
	}
	defer file.Close()

	result := &HashResult{}

	if arg.HashSelect == nil {
		result = &HashResult{
			md5:    md5.New(),
			sha1:   sha1.New(),
			sha256: sha256.New(),
		}
	} else {
		for _, hash_opt := range arg.HashSelect {
			switch hash_opt {
			case "sha256", "SHA256":
				result.sha256 = sha256.New()
			case "sha1", "SHA1":
				result.sha1 = sha1.New()
			case "md5", "MD5":
				result.md5 = md5.New()
			default:
				scope.Log("hashselect option %s not recognized (should be md5, sha1, sha256)",
					hash_opt)
				return vfilter.Null{}
			}
		}
	}

	var count uint64
	for {
		select {
		case <-ctx.Done():
			return vfilter.Null{}

		default:
			n, err := file.Read(buf)

			count += uint64(n)
			if count > arg.MaxSize {
				DeduplicatedLog(
					ctx, scope, "Hash of %v aborted due to exceeding size", arg.Path)
				n = 0
			}

			// We are done!
			if n == 0 || err == io.EOF {
				if n == 0 {
					if result.md5 != nil {
						result.MD5 = fmt.Sprintf(
							"%x", result.md5.Sum(nil))
					}

					if result.sha1 != nil {
						result.SHA1 = fmt.Sprintf(
							"%x", result.sha1.Sum(nil))
					}

					if result.sha256 != nil {
						result.SHA256 = fmt.Sprintf(
							"%x", result.sha256.Sum(nil))
					}
					return result.ToDict()
				}

			} else if err != nil {
				scope.Log("hash: %v", err)
				return vfilter.Null{}
			}

			if result.md5 != nil {
				_, _ = result.md5.Write(buf[:n])
			}

			if result.sha1 != nil {
				_, _ = result.sha1.Write(buf[:n])
			}

			if result.sha256 != nil {
				_, _ = result.sha256.Write(buf[:n])
			}

			// Charge an op for each buffer we read
			scope.ChargeOp()
		}
	}
}

func (self HashFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "hash",
		Doc:      "Calculate the hash of a file.",
		ArgType:  type_map.AddType(scope, &HashFunctionArgs{}),
		Version:  3,
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&HashFunction{})
}
