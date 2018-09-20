package functions

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"

	glob "www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
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
	Path     string `vfilter:"required,field=path"`
	Accessor string `vfilter:"optional,field=accessor"`
}

// The hash fuction calculates a hash of a file. It may be expensive
// so we make it cancelllable.
type HashFunction struct{}

func (self *HashFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &HashFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("hash: %s", err.Error())
		return false
	}

	buf := make([]byte, 4*1024*1024) // 4Mb chunks
	fs := glob.GetAccessor(arg.Accessor, ctx)
	file, err := fs.Open(arg.Path)
	if err != nil {
		scope.Log(err.Error())
		return false
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
			if err == io.EOF {
				result.MD5 = fmt.Sprintf(
					"%x", result.md5.Sum(nil))
				result.SHA1 = fmt.Sprintf(
					"%x", result.sha1.Sum(nil))
				result.SHA256 = fmt.Sprintf(
					"%x", result.sha256.Sum(nil))

				return result

			} else if err != nil {
				scope.Log(err.Error())
				return vfilter.Null{}
			}

			result.md5.Write(buf[:n])
			result.sha1.Write(buf[:n])
			result.sha256.Write(buf[:n])
		}
	}
}

func (self HashFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "hash",
		Doc:     "Calculate the hash of a file.",
		ArgType: type_map.AddType(&HashFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&HashFunction{})
}
