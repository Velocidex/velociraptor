package logging_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type TestStruct struct {
	Int1    uint64
	Message string
}

type LoggingTestSuite struct {
	test_utils.TestSuite
}

func (self *LoggingTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	self.LoadArtifacts(`name: Server.Audit.Logs
type: SERVER_EVENT
`)
}

func (self *LoggingTestSuite) TestAuditLog() {
	t := self.T()

	dir, err := tempfile.TempDir("file_store_test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	closer := utils.MockTime(utils.NewMockClock(time.Unix(1602103388, 0)))
	defer closer()

	config_obj := config.GetDefaultConfig()
	config_obj.Logging.OutputDirectory = dir

	err = logging.InitLogging(config_obj)
	assert.NoError(t, err)

	services.LogAudit(context.Background(),
		config_obj, "Principal", "SomeOperation",
		ordereddict.NewDict().
			Set("SomeField", 1).
			Set("NestedField", ordereddict.NewDict().
				Set("Field1", 1).
				Set("Field2", 3)).
			Set("StructField", &TestStruct{
				Int1:    54,
				Message: "Hello",
			}).
			Set("err", http.StatusUnauthorized))

	// Read the audit log
	fd, err := os.Open(filepath.Join(dir, "VelociraptorAudit_info.log"))
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err)

	goldie.Assert(t, "TestAuditLog", data)
}

func TestLogging(t *testing.T) {
	suite.Run(t, &LoggingTestSuite{})
}
