package readers

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type TestSuite struct {
	suite.Suite
}

func (self *TestSuite) TestPagedReader() {
	scope := vql_subsystem.MakeScope()

	scope.AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache()).
		Set(constants.SCOPE_ROOT, scope))

	pool := &ReaderPool{
		lru: cache.NewLRUCache(5),
	}
	vql_subsystem.CacheSet(scope, READERS_CACHE, pool)
	vql_subsystem.GetRootScope(scope).AddDestructor(pool.Close)

	tmp_dir, err := ioutil.TempDir("", "tmp")
	assert.NoError(self.T(), err)

	defer os.RemoveAll(tmp_dir)

	// Create 10 files with data
	buff := make([]byte, 4)
	filenames := make([]string, 0, 10)

	for i := 0; i < 10; i++ {
		filename := fmt.Sprintf("%s/Test%x.txt", tmp_dir, i)
		filenames = append(filenames, filename)

		out_fd, err := os.OpenFile(
			filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		assert.NoError(self.T(), err)

		binary.LittleEndian.PutUint32(buff, uint32(i))
		out_fd.Write(buff)
		out_fd.Close()
	}

	// Open 10 paged readers - This should close 5
	readers := make([]*AccessorReader, 0, 10)
	for i := 0; i < 10; i++ {
		reader := NewPagedReader(scope, "file", filenames[i])
		reader.ReadAt(buff, 0)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(i))
		readers = append(readers, reader)
	}

	for i := 0; i < 10; i++ {
		reader := NewPagedReader(scope, "file", filenames[i])
		reader.ReadAt(buff, 0)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(i))
	}

	// Open the same reader 10 time returns from the cache.
	for i := 0; i < 10; i++ {
		reader := NewPagedReader(scope, "file", filenames[1])
		reader.ReadAt(buff, 0)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))
	}

	// Close the scope - this should close all the pool
	scope.Close()

	// No outstanding readers
	assert.Equal(self.T(), int64(0), pool.lru.Size())
}

func TestReaders(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
