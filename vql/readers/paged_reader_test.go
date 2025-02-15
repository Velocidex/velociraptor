package readers

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

type TestSuite struct {
	suite.Suite
	scope     vfilter.Scope
	tmp_dir   string
	filenames []*accessors.OSPath
	pool      *ReaderPool
}

func (self *TestSuite) SetupTest() {
	self.scope = vql_subsystem.MakeScope()
	self.scope.AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}).
		Set(constants.SCOPE_ROOT, self.scope))

	// Make a very small pool
	self.pool = GetReaderPool(self.scope, 5)

	var err error
	self.tmp_dir, err = tempfile.TempDir("tmp")
	assert.NoError(self.T(), err)

	// Create 10 files with data
	buff := make([]byte, 4)
	self.filenames = make([]*accessors.OSPath, 0, 10)
	accessor, err := accessors.GetAccessor("file", self.scope)
	assert.NoError(self.T(), err)

	for i := 0; i < 10; i++ {
		filename, err := accessor.ParsePath(self.tmp_dir)
		assert.NoError(self.T(), err)

		file := filename.Append(fmt.Sprintf("Test%x.txt", i))
		self.filenames = append(self.filenames, file)

		out_fd, err := os.OpenFile(
			file.String(), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
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
		reader, err := NewAccessorReader(
			self.scope, "file", self.filenames[i], 100)
		assert.NoError(self.T(), err)
		_, err = reader.ReadAt(buff, 0)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(i))
		readers = append(readers, reader)
	}

	for i := 0; i < 10; i++ {
		reader, err := NewAccessorReader(self.scope, "file", self.filenames[i], 100)
		assert.NoError(self.T(), err)
		_, err = reader.ReadAt(buff, 0)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(i))
	}

	// Open the same reader 10 time returns from the cache.
	for i := 0; i < 10; i++ {
		reader, err := NewAccessorReader(self.scope, "file", self.filenames[1], 100)
		assert.NoError(self.T(), err)

		_, err = reader.ReadAt(buff, 0)
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))
	}

	// Make sure that it is ok to close the reader at any time -
	// the next read will be valid.
	reader, err := NewAccessorReader(self.scope, "file", self.filenames[1], 100)
	assert.NoError(self.T(), err)

	for i := 0; i < 10; i++ {
		reader.Close()
		_, err = reader.ReadAt(buff, 0)
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))
	}

	// Make the reader's timeout very short.
	reader.SetLifetime(10 * time.Millisecond)
	reader.Close()
	_, err = reader.ReadAt(buff, 0)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))

	// Wait here until the reader closes itself by itself.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		reader.mu.Lock()
		defer reader.mu.Unlock()

		return reader.reader == nil
	})

	// Next read still works.
	_, err = reader.ReadAt(buff, 0)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), binary.LittleEndian.Uint32(buff), uint32(1))

	// Close the scope - this should close all the pool
	self.scope.Close()

	// Destoying the scope should close the readers.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		reader.mu.Lock()
		defer reader.mu.Unlock()

		return reader.reader == nil
	})

	// No outstanding readers
	metrics := self.pool.lru.GetMetrics()
	assert.Equal(self.T(), int64(0), metrics.Size)

}

func TestReaders(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
