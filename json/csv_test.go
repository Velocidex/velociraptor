package json_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type CSVUtilsTestSuite struct {
	test_utils.TestSuite
	client_id, flow_id string
}

func (self *CSVUtilsTestSuite) TestCSVUtils() {
	path_manager := artifacts.NewArtifactPathManagerWithMode(
		self.ConfigObj, self.client_id, self.flow_id,
		"Artifact", paths.MODE_CLIENT)

	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	writer, err := result_sets.NewResultSetWriter(file_store_factory,
		path_manager.Path(), json.DefaultEncOpts(),
		utils.SyncCompleter, result_sets.TruncateMode)
	assert.NoError(self.T(), err)

	// Create a sample result set
	for i := 0; i < 10; i++ {
		writer.Write(ordereddict.NewDict().
			Set("Col With \" 1",
				fmt.Sprintf("Value \" with quotes: %d", i)).
			Set("Object", ordereddict.NewDict().Set("Foo", 1)).
			Set("Col2", i*2))
	}

	writer.Close()

	reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
	assert.NoError(self.T(), err)
	defer reader.Close()

	json_chan, err := reader.JSON(self.Ctx)
	assert.NoError(self.T(), err)

	json_buffer := &bytes.Buffer{}
	csv_buffer := &bytes.Buffer{}

	json.ConvertJSONL(json_chan, json_buffer, csv_buffer,
		ordereddict.NewDict().
			Set("ClientId", "C.123").
			Set("HuntId", "H.123"))

	golden := fmt.Sprintf("JSONL:\n------\n%v\nCSV:\n------\n%v\n",
		string(json_buffer.Bytes()), string(csv_buffer.Bytes()))

	goldie.Assert(self.T(), "TestCSVUtils", []byte(golden))

}

func TestCSVUtils(t *testing.T) {
	suite.Run(t, &CSVUtilsTestSuite{
		client_id: "C.1234",
		flow_id:   "F.123",
	})
}
