// Implement transparent asset decompression and caching.

// Most modern browsers can negotiate compression transfer with no
// issues. In this case we just deliver the compressed assets directly
// saving on both bandwidth and CPU cycles.

// However older browsers may not support brotli compression, so we
// need to decompress the asset for them and cache it for a short
// time.

package api

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"github.com/andybalholm/brotli"
	errors "github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	brotliDecompressionCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gui_asset_decompression_count",
		Help: "Number of times the GUI was forced to decompressed assets for browsers that do not support brotli compression.",
	})
)

type memFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func (f *memFileInfo) Name() string       { return f.name }
func (f *memFileInfo) Size() int64        { return f.size }
func (f *memFileInfo) Mode() os.FileMode  { return f.mode }
func (f *memFileInfo) ModTime() time.Time { return f.modTime }
func (f *memFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f *memFileInfo) Sys() interface{}   { return nil }

type brotliBuffer struct {
	*bytes.Reader
	name string
	size int64
}

func (self *brotliBuffer) Close() error {
	return nil
}

func (self *brotliBuffer) Readdir(count int) ([]fs.FileInfo, error) {
	return nil, errors.New("Not Implemented")
}

func (self *brotliBuffer) Stat() (fs.FileInfo, error) {
	return &memFileInfo{
		name: self.name,
		size: self.size,
		mode: 0644,
	}, nil
}

type CachedFilesystem struct {
	http.FileSystem
	lru *ttlcache.Cache
}

func (self *CachedFilesystem) getCachedBytes(name string) ([]byte, error) {
	cached_any, err := self.lru.Get(name)
	if err != nil {
		return nil, err
	}

	cached, ok := cached_any.([]byte)
	if !ok {
		return nil, errors.New("Invalid cached item")
	}
	return cached, nil
}

func (self *CachedFilesystem) Open(name string) (http.File, error) {
	// We do not support gz files at all - it is either brotli or
	// uncompressed.
	if strings.HasSuffix(name, ".gz") {
		return nil, services.OrgNotFoundError
	}

	fd, err := self.FileSystem.Open(name)
	if err != nil {
		// If there is not brotli file, it is just not there.
		if strings.HasSuffix(name, ".br") {
			return nil, services.OrgNotFoundError
		}

		// Check if a compressed .br file exists
		fd, err := self.FileSystem.Open(name + ".br")
		if err == nil {
			cached, err := self.getCachedBytes(name)
			if err == nil {
				return &brotliBuffer{
					Reader: bytes.NewReader(cached),
					name:   name,
					size:   int64(len(cached)),
				}, nil
			}

			out_fd := &bytes.Buffer{}
			n, err := io.Copy(out_fd, brotli.NewReader(fd))
			if err != nil {
				return nil, err
			}

			brotliDecompressionCounter.Inc()

			// Cache for next time.
			err = self.lru.Set(name, out_fd.Bytes())

			return &brotliBuffer{
				Reader: bytes.NewReader(out_fd.Bytes()),
				name:   name,
				size:   n,
			}, err
		}
	}
	return fd, err
}

func (self *CachedFilesystem) Exists(path string) bool {
	fd, err := self.FileSystem.Open(path)
	if err != nil {
		return false
	}
	fd.Close()
	return true
}

func NewCachedFilesystem(
	ctx context.Context, fs http.FileSystem) *CachedFilesystem {
	result := &CachedFilesystem{
		FileSystem: fs,
		lru:        ttlcache.NewCache(),
	}

	_ = result.lru.SetTTL(10 * time.Minute)
	result.lru.SkipTTLExtensionOnHit(true)

	go func() {
		<-ctx.Done()
		result.lru.Close()
	}()

	return result
}
