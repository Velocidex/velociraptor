package vql

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	glob "www.velocidex.com/golang/velociraptor/glob"
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

// The hash fuction calculates a hash of a file. It may be expensive
// so we make it cancelllable.
type HashFunction struct{}

func (self *HashFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	path, pres := vfilter.ExtractString("path", args)
	if !pres {
		scope.Log("Arg path not specified")
		return false
	}

	buf := make([]byte, 4*1024*1024) // 4Mb chunks
	fs := glob.OSFileSystemAccessor{}
	file, err := fs.Open(*path)
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

func (self HashFunction) Name() string {
	return "hash"
}

func init() {
	exportedFunctions = append(exportedFunctions, &HashFunction{})
}
