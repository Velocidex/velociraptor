package test_utils

import (
	"io/ioutil"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/require"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/utils"
)

func GetMemoryFileStore(
	t *testing.T,
	config_obj *config_proto.Config) *memory.MemoryFileStore {
	file_store_factory, ok := file_store.GetFileStore(config_obj).(*memory.MemoryFileStore)
	require.True(t, ok)

	return file_store_factory
}

func GetMemoryDataStore(
	t *testing.T,
	config_obj *config_proto.Config) *datastore.TestDataStore {
	db, err := datastore.GetDB(config_obj)
	require.NoError(t, err)

	return db.(*datastore.TestDataStore)
}

func FileReadAll(t *testing.T, config_obj *config_proto.Config, vfs_path string) string {
	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(vfs_path)
	require.NoError(t, err)

	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	require.NoError(t, err)

	return string(data)
}

func FileReadRows(t *testing.T, config_obj *config_proto.Config,
	vfs_path string) []*ordereddict.Dict {

	data := FileReadAll(t, config_obj, vfs_path)
	res, err := utils.ParseJsonToDicts([]byte(data))
	require.NoError(t, err)

	return res
}
