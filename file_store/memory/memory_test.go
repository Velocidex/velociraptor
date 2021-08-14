package memory

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/tests"
)

type MemoryTestSuite struct {
	*tests.FileStoreTestSuite

	file_store *MemoryFileStore
}

func (self *MemoryTestSuite) SetupTest() {
	self.file_store.Clear()
}

func TestMemoeyFileStore(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	file_store := NewMemoryFileStore(config_obj)
	suite.Run(t, &MemoryTestSuite{
		FileStoreTestSuite: tests.NewFileStoreTestSuite(config_obj, file_store),
		file_store:         file_store,
	})
}
