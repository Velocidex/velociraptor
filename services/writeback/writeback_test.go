package writeback_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/Velocidex/yaml/v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func readWritebackFile(t *testing.T, filename string) (*config_proto.Writeback, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	result := &config_proto.Writeback{}
	err = yaml.Unmarshal(data, result)
	return result, err
}

func corruptWritebackFile(t *testing.T, filename string) {
	err := ioutil.WriteFile(filename, []byte("{{{{"), 0664)
	assert.NoError(t, err)
}

func TestWriteback(t *testing.T) {
	level2_suffix := "l2"

	// Set a tempfile for the writeback.
	writeback_fd, err := tempfile.TempFile("")
	assert.NoError(t, err)
	writeback_fd.Close()

	defer os.Remove(writeback_fd.Name())
	defer os.Remove(writeback_fd.Name() + level2_suffix)

	config_obj := config.GetDefaultConfig()
	config_obj.Client.WritebackWindows = writeback_fd.Name()
	config_obj.Client.WritebackLinux = writeback_fd.Name()
	config_obj.Client.WritebackDarwin = writeback_fd.Name()
	config_obj.Client.Level2WritebackSuffix = level2_suffix

	// Start the service to read from this file.
	writeback_service := writeback.GetWritebackService()
	writeback_service.LoadWriteback(config_obj)

	// Write something to the primary writeback file.
	err = writeback_service.MutateWriteback(config_obj,
		func(wb *config_proto.Writeback) error {
			wb.PrivateKey = "Private"
			return writeback.WritebackUpdateLevel1
		})
	assert.NoError(t, err)

	wb, err := readWritebackFile(t, writeback_fd.Name())
	assert.NoError(t, err)
	assert.Equal(t, "Private", wb.PrivateKey)

	// Corrupt the file.
	corruptWritebackFile(t, writeback_fd.Name())

	// Query the writeback manager for it - still should have valid
	// data.
	wb, err = writeback_service.GetWriteback(config_obj)
	assert.NoError(t, err)
	assert.Equal(t, "Private", wb.PrivateKey)

	// l2 file is not present
	_, err = readWritebackFile(t, writeback_fd.Name()+level2_suffix)
	assert.Error(t, err)

	// Check for writing to l2 file
	err = writeback_service.MutateWriteback(config_obj,
		func(wb *config_proto.Writeback) error {
			wb.EventQueries = &actions_proto.VQLEventTable{
				Version: 5,
			}
			return writeback.WritebackUpdateLevel2
		})
	assert.NoError(t, err)

	// l2 file appeared
	wb, err = readWritebackFile(t, writeback_fd.Name()+level2_suffix)
	assert.NoError(t, err)
	assert.Equal(t, "Private", wb.PrivateKey)
	assert.Equal(t, uint64(5), wb.EventQueries.Version)

	// Can the service be recovered from the l2 writeback alone?
	corruptWritebackFile(t, writeback_fd.Name())

	// This will load the writeback from level 1 file, and since it is
	// corrupted, level 2 file. Level 2 file has all the info from
	// level 1 file.
	writeback_service.(*writeback.WritebackService).Reset(config_obj)

	wb, err = writeback_service.GetWriteback(config_obj)
	assert.NoError(t, err)
	assert.Equal(t, "Private", wb.PrivateKey)

}
