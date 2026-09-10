package writeback_test

import (
	"testing"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// The pool client uses in-memory writebacks so no files are written
// to disk. The writeback service dispatches on the writeback location
// so each pool client gets its own in-memory writeback.
func TestMemoryWriteback(t *testing.T) {
	config_obj := &config_proto.Config{
		Client: &config_proto.ClientConfig{
			WritebackLinux:   "memory://pool_client.yaml.0",
			WritebackWindows: "memory://pool_client.yaml.0",
			WritebackDarwin:  "memory://pool_client.yaml.0",
		},
	}

	writeback_service := writeback.GetWritebackService()
	err := writeback_service.LoadWriteback(config_obj)
	assert.NoError(t, err)

	// Write a private key to the in-memory writeback.
	err = writeback_service.MutateWriteback(config_obj,
		func(wb *config_proto.Writeback) error {
			wb.PrivateKey = "Private"
			wb.ClientId = "C.1234"
			return writeback.WritebackUpdateLevel1
		})
	assert.NoError(t, err)

	// Read it back.
	wb, err := writeback_service.GetWriteback(config_obj)
	assert.NoError(t, err)
	assert.Equal(t, "Private", wb.PrivateKey)
	assert.Equal(t, "C.1234", wb.ClientId)

	// A different writeback location gets a different (empty)
	// in-memory writeback.
	config_obj2 := &config_proto.Config{
		Client: &config_proto.ClientConfig{
			WritebackLinux:   "memory://pool_client.yaml.1",
			WritebackWindows: "memory://pool_client.yaml.1",
			WritebackDarwin:  "memory://pool_client.yaml.1",
		},
	}

	err = writeback_service.LoadWriteback(config_obj2)
	assert.NoError(t, err)

	wb2, err := writeback_service.GetWriteback(config_obj2)
	assert.NoError(t, err)
	assert.Equal(t, "", wb2.PrivateKey)
	assert.Equal(t, "", wb2.ClientId)
}
