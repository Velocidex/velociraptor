package logging_test

import (
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type TestStruct struct {
	Int1    uint64
	Message string
}

func TestAuditLog(t *testing.T) {
	dir, err := ioutil.TempDir("", "file_store_test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	closer := utils.MockTime(&utils.MockClock{
		MockNow: time.Unix(1602103388, 0),
	})
	defer closer()

	config_obj := config.GetDefaultConfig()
	config_obj.Logging.OutputDirectory = dir

	err = logging.InitLogging(config_obj)
	assert.NoError(t, err)

	logging.LogAudit(config_obj, "Principal", "SomeOperation",
		logrus.Fields{
			"SomeField": 1,
			"NestedField": ordereddict.NewDict().
				Set("Field1", 1).
				Set("Field2", 3),
			"StructField": &TestStruct{
				Int1:    54,
				Message: "Hello",
			},
			"err": http.StatusUnauthorized,
		})

	utils.DlvBreak()

	// Read the audit log
	fd, err := os.Open(filepath.Join(dir, "VelociraptorAudit_info.log"))
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err)

	goldie.Assert(t, "TestAuditLog", data)
}
