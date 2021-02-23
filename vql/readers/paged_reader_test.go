// +build !windows

package readers

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/vfilter"
)

type TestSuite struct {
	suite.Suite
	scope     vfilter.Scope
	tmp_dir   string
	filenames []string
	pool      *ReaderPool
}

func (self *TestSuite) SetupTest() {
	self.scope = vql_subsystem.MakeScope()
	self.scope.AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache()).
		Set(constants.SCOPE_ROOT, self.scope))

	// Make a very small pool
	self.pool = GetReaderPool(self.scope, 5)

	var err error
	self.tmp_dir, err = ioutil.TempDir("", "tmp")
	assert.NoError(self.T(), err)

	// Create 10 files with data
	buff := make([]byte, 4)
	self.filenames = make([]string, 0, 10)

	for i := 0; i < 10; i++ {
		filename := fmt.Sprintf("%s/Test%x.txt", self.tmp_dir, i)
		self.filenames = append(self.filenames, filename)

		out_fd, err := os.OpenFile(
			filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		assert.NoError(self.T(), err)

		binary.LittleEndian.PutUint32(buff, uint32(i))
		out_fd.Write(buff)
		out_fd.Close()
	}

}

func (self *TestSuite) TearDownTest() {
	self.scope.Close()
	os.RemoveAll(self.tmp_dir)
}

func (self *TestSuite) TestPagedReader() {
	// Open 10 paged readers - This should close 5
	readers := make([]*AccessorReader, 0, 10)
	buff := make([]byte, 4)

	for i := 0; i < 10; i++ {
		reader := NewPagedReader(self.scope, "file", self.filenames[i])
		reader.ReadAt(buff, 0)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(i))
		readers = append(readers, reader)
	}

	for i := 0; i < 10; i++ {
		reader := NewPagedReader(self.scope, "file", self.filenames[i])
		reader.ReadAt(buff, 0)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(i))
	}

	// Open the same reader 10 time returns from the cache.
	for i := 0; i < 10; i++ {
		reader := NewPagedReader(self.scope, "file", self.filenames[1])
		reader.ReadAt(buff, 0)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))
	}

	// Make sure that it is ok to close the reader at any time -
	// the next read will be valid.
	reader := NewPagedReader(self.scope, "file", self.filenames[1])
	for i := 0; i < 10; i++ {
		reader.Close()
		reader.ReadAt(buff, 0)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))
	}

	// Make the reader's timeout very short.
	reader.Lifetime = 10 * time.Millisecond
	reader.Close()
	reader.ReadAt(buff, 0)
	assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))

	// Wait here until the reader closes itself by itself.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return reader.reader == nil
	})

	// Next read still works.
	reader.ReadAt(buff, 0)
	assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))

	// Close the scope - this should close all the pool
	self.scope.Close()

	// Destoying the scope should close the readers.
	assert.Nil(self.T(), reader.reader)

	// No outstanding readers
	assert.Equal(self.T(), int64(0), self.pool.lru.Size())

}

func TestReaders(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
