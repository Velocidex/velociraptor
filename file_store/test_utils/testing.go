package test_utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
)

func GetMemoryFileStore(
	t *testing.T,
	config_obj *config_proto.Config) *memory.MemoryFileStore {
	file_store_factory, ok := file_store.GetFileStore(config_obj).(*memory.MemoryFileStore)
	require.True(t, ok)

	return file_store_factory
}
