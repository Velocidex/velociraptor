package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

const (
	writeback_path = "fixtures/writeback.yaml"
)

type CrytpoStoreTestSuite struct {
	test_utils.TestSuite

	tmp_dir string
}

func (self *CrytpoStoreTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services = &config_proto.ServerServicesConfig{
		IndexServer:    true,
		ClientInfo:     true,
		JournalService: true,
	}

	self.ConfigObj.Client.WritebackLinux = writeback_path
	self.ConfigObj.Client.WritebackWindows = writeback_path

	self.TestSuite.SetupTest()

	writeback_service := writeback.GetWritebackService()
	writeback_service.LoadWriteback(self.ConfigObj)

	err := utils.VerifyConfig(self.ConfigObj)
	assert.NoError(self.T(), err)

	self.tmp_dir, err = tempfile.TempDir("tmp")
	assert.NoError(self.T(), err)
}

func (self *CrytpoStoreTestSuite) TearDownTest() {
	os.RemoveAll(self.tmp_dir)
}

func (self *CrytpoStoreTestSuite) TestWritingAndReading() {
	self.testWriting()
	self.testReading()
}

func (self *CrytpoStoreTestSuite) testWriting() {
	output := filepath.Join(self.tmp_dir, "test.bin")

	// Initial state no server connection.
	SetCurrentServerPem(nil)

	fd, err := NewCryptoFileWriter(self.Ctx, self.ConfigObj, 10000, output)
	assert.NoError(self.T(), err)
	defer fd.Close()

	// This should fail: Add a message before server pem is known.
	fd.AddMessage(&crypto_proto.VeloMessage{LogMessage: &crypto_proto.LogMessage{
		Jsonl: "First Message",
	}})
	err = fd.Flush(KEEP_ON_ERROR)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Server PEM not initialized")

	// However the message should remain on the queue for flushing
	// next.

	// Initialize the server PEM manually. Usually this will be done
	// by the communicator once we reach the server and read it's
	// certificate.
	SetCurrentServerPem([]byte(self.ConfigObj.Frontend.Certificate))

	for i := 0; i < 10; i++ {
		fd.AddMessage(&crypto_proto.VeloMessage{
			LogMessage: &crypto_proto.LogMessage{
				Jsonl: fmt.Sprintf("{\"Message\": \"Hello world %v\"}", i),
			},
		})
	}

	// Flush the data to disk
	err = fd.Flush(!KEEP_ON_ERROR)
	assert.NoError(self.T(), err)

	// Make sure the messages were removed.
	fd.AddMessage(&crypto_proto.VeloMessage{LogMessage: &crypto_proto.LogMessage{
		Jsonl: "Final",
	}})
	err = fd.Flush(KEEP_ON_ERROR)
	assert.NoError(self.T(), err)

	// The file data will be inspected by the testReading() method.
}

func (self *CrytpoStoreTestSuite) testReading() {
	fd, err := os.Open(filepath.Join(self.tmp_dir, "test.bin"))
	assert.NoError(self.T(), err)

	reader, err := NewCryptoFileReader(self.Ctx, self.ConfigObj, fd)
	assert.NoError(self.T(), err)
	defer reader.Close()

	var golden []*crypto_proto.VeloMessage
	for msg := range reader.Parse(self.Ctx) {
		golden = append(golden, msg)
	}

	goldie.Assert(self.T(), "TestWritingAndReading",
		json.MustMarshalIndent(golden))
}

func TestCryptoStore(t *testing.T) {
	suite.Run(t, new(CrytpoStoreTestSuite))
}
