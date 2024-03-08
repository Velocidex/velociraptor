package tools

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/filesystem"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	"www.velocidex.com/golang/velociraptor/accessors/s3"
	_ "www.velocidex.com/golang/velociraptor/accessors/s3"
)

/*
   This test must be ran manually:

   To fake out the real S3 service I use minio from https://min.io/

   Download and unpack to a directory and do the following setup:

   # Start the local minio server
   MINIO_ROOT_USER=admin MINIO_ROOT_PASSWORD=password ./minio server /tmp/minio --console-address ":9001" --address ":4566"

   # Create the velociraptor bucket
   ./mc alias set myminio http://127.0.0.1:4566 admin password
   ./mc mb myminio/velociraptor

   The below constants are designed to connect to that test instance.

   Enable the below test by defining the env variable ENABLE_MINIO:

   ENABLE_MINIO=1 go test ./vql/tools
*/

const (
	username = "admin"
	password = "password"
	endpoint = "http://127.0.0.1:4566/"
	bucket   = "velociraptor"
)

type S3TestSuite struct {
	test_utils.TestSuite
}

func (self *S3TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.TestSuite.SetupTest()
}

func (self *S3TestSuite) TestUpload() {
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict().
			Set("S3_CREDENTIALS", ordereddict.NewDict().
				Set("endpoint", endpoint).
				Set("credentials_key", username).
				Set("credentials_secret", password)),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	// Upload 10 files to the bucket. Credentials come from the scope
	// env.
	for i := 0; i < 10; i++ {
		filename := fmt.Sprintf("hello_%d.txt", i)
		row := (S3UploadFunction{}).Call(self.Ctx, scope,
			ordereddict.NewDict().
				Set("accessor", "data").
				Set("file", "hello world").
				Set("bucket", bucket).
				Set("name", filename))
		line := json.MustMarshalString(row)
		assert.Contains(self.T(), line, filename)
	}

	// Now glob the bucket to see if all the files are there.
	// Shrink the page size to 2 to force us to page a lot.
	s3.SetPageSize(2)

	result := []string{}
	result_regex := regexp.MustCompile("/velociraptor/hello_\\d+.txt")
	snapshot := vtesting.GetMetrics(self.T(), "s3_ops_list_objects")

	// We upload 10 files to the bucket.
	for row := range (filesystem.GlobPlugin{}).Call(self.Ctx, scope,
		ordereddict.NewDict().
			Set("globs", "/velociraptor/*").
			Set("accessor", "s3")) {
		line, err := accessors.MarshalGlobFileInfo(
			row, json.DefaultEncOpts())
		assert.NoError(self.T(), err)

		if result_regex.MatchString(string(line)) {
			result = append(result, string(line))
		}
	}
	assert.Equal(self.T(), len(result), 10)

	// Check that we have to page a lot
	metrics := vtesting.GetMetricsDifference(
		self.T(), "s3_ops_list_objects", snapshot)
	list_op_count, _ := metrics.GetInt64("s3_ops_list_objects")

	// We have to do at least 5 pages to get all the data (10 items at
	// 2 per page).
	assert.True(self.T(), list_op_count > 5)
}

func TestS3Plugin(t *testing.T) {
	value := os.Getenv("ENABLE_MINIO")
	if value == "" {
		t.Skip("Test skipped because ENABLE_MINIO is not defined. Set up MINIO according to the above instructions then try again.")
	}
	suite.Run(t, &S3TestSuite{})
}
