package functions

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

func compressWith(t *testing.T, w io.WriteCloser, data string) {
	_, err := w.Write([]byte(data))
	assert.NoError(t, err)
	assert.NoError(t, w.Close())
}

func TestDecompressFunctions(t *testing.T) {
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, "", 0))
	defer scope.Close()

	plain := `{"training_allowed": true}` + strings.Repeat(" ", 1000)

	br := &bytes.Buffer{}
	compressWith(t, brotli.NewWriter(br), plain)

	zs := &bytes.Buffer{}
	enc, err := zstd.NewWriter(zs)
	assert.NoError(t, err)
	compressWith(t, enc, plain)

	for _, tc := range []struct {
		name       string
		fn         types.FunctionInterface
		compressed string
	}{
		{"brotli", &BrotliDecompress{}, br.String()},
		{"zstd", &ZstdDecompress{}, zs.String()},
	} {
		call := func(data string, max_bytes int64) vfilter.Any {
			args := ordereddict.NewDict().Set("string", data)
			if max_bytes > 0 {
				args.Set("max_bytes", max_bytes)
			}
			return tc.fn.Call(ctx, scope, args)
		}

		assert.Equal(t, plain, call(tc.compressed, 0), tc.name)

		// Output is capped at max_bytes.
		assert.Equal(t, plain[:10], call(tc.compressed, 10), tc.name)

		// Invalid input returns NULL rather than garbage.
		assert.Equal(t, vfilter.Null{}, call("not compressed data", 0), tc.name)
	}
}
