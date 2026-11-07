/*
Velociraptor - Dig Deeper
Copyright (C) 2026 Velocidex Innovations.

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
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DecompressArgs struct {
	String   string `vfilter:"required,field=string,doc=Data to decompress"`
	MaxBytes int64  `vfilter:"optional,field=max_bytes,doc=Maximum length of bytes to read into memory"`
}

// Decompress a string using the reader returned by new_reader. Data
// past MaxBytes is dropped so a hostile stream can not exhaust memory.
func decompressString(ctx context.Context,
	scope vfilter.Scope, args *ordereddict.Dict, name string,
	new_reader func(r io.Reader) (io.Reader, func(), error)) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, name, args)()

	arg := &DecompressArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("%v: %v", name, err)
		return vfilter.Null{}
	}

	if arg.MaxBytes <= 0 || arg.MaxBytes > constants.MAX_MEMORY_LARGE {
		arg.MaxBytes = constants.MAX_MEMORY_LARGE
	}

	r, closer, err := new_reader(strings.NewReader(arg.String))
	if err != nil {
		scope.Log("%v: %v", name, err)
		return vfilter.Null{}
	}
	defer closer()

	var out bytes.Buffer
	_, err = out.ReadFrom(io.LimitReader(r, arg.MaxBytes))
	if err != nil {
		scope.Log("%v: %v", name, err)
		return vfilter.Null{}
	}

	return out.String()
}

type BrotliDecompress struct{}

func (self *BrotliDecompress) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	return decompressString(ctx, scope, args, "brotli_decompress",
		func(r io.Reader) (io.Reader, func(), error) {
			return brotli.NewReader(r), func() {}, nil
		})
}

func (self BrotliDecompress) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "brotli_decompress",
		Doc:     "Decompress a brotli-compressed block of data.",
		ArgType: type_map.AddType(scope, &DecompressArgs{}),
	}
}

type ZstdDecompress struct{}

func (self *ZstdDecompress) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	return decompressString(ctx, scope, args, "zstd_decompress",
		func(r io.Reader) (io.Reader, func(), error) {
			// Use a single goroutine: these blobs are small and we
			// do not want background decoders.
			d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1))
			if err != nil {
				return nil, nil, err
			}
			return d, d.Close, nil
		})
}

func (self ZstdDecompress) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "zstd_decompress",
		Doc:     "Decompress a zstd-compressed block of data.",
		ArgType: type_map.AddType(scope, &DecompressArgs{}),
	}
}

type XzDecompress struct{}

func (self *XzDecompress) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	return decompressString(ctx, scope, args, "xz_decompress",
		func(r io.Reader) (io.Reader, func(), error) {
			d, err := xz.NewReader(r)
			if err != nil {
				return nil, nil, err
			}
			return d, func() {}, nil
		})
}

func (self XzDecompress) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "xz_decompress",
		Doc:     "Decompress an xz-compressed block of data.",
		ArgType: type_map.AddType(scope, &DecompressArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&BrotliDecompress{})
	vql_subsystem.RegisterFunction(&ZstdDecompress{})
	vql_subsystem.RegisterFunction(&XzDecompress{})
}
