package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type DownloadPublicTestSuite struct {
	test_utils.TestSuite
	dir string
}

func (self *DownloadPublicTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()

	var err error
	self.dir, err = tempfile.TempDir("download_public_test")
	assert.NoError(self.T(), err)

	file_store.ClearGlobalFilestore()

	self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	self.ConfigObj.Datastore.FilestoreDirectory = self.dir
	self.ConfigObj.Datastore.Location = self.dir

	self.TestSuite.SetupTest()
}

func (self *DownloadPublicTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	file_store.ClearGlobalFilestore()

	_ = os.RemoveAll(self.dir)
}

func (self *DownloadPublicTestSuite) TestDownloadOrgFormUpload() {
	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	_, err = org_manager.CreateNewOrg("Test Org", "O123", services.RandomNonce)
	assert.NoError(self.T(), err)

	org_config_obj, err := org_manager.GetOrgConfig("O123")
	assert.NoError(self.T(), err)

	path_manager := paths.NewFormUploadPathManager(org_config_obj, "rules.yar")
	pathspec, file_store_factory, err := path_manager.Path()
	assert.NoError(self.T(), err)

	writer, err := file_store_factory.WriteFile(pathspec)
	assert.NoError(self.T(), err)

	_, err = writer.Write([]byte("rule X { condition: true }"))
	assert.NoError(self.T(), err)
	assert.NoError(self.T(), writer.Close())

	handler := downloadPublic(self.ConfigObj, [][]string{{"public"}})

	// The URL handed out for the upload resolves to the org's file.
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET",
		path_manager.URL(), nil))
	assert.Equal(self.T(), http.StatusOK, recorder.Code)
	assert.Equal(self.T(), "rule X { condition: true }", recorder.Body.String())

	// Without the org id the root org's filestore is used.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET",
		pathspec.AsClientPath(), nil))
	assert.Equal(self.T(), http.StatusNotFound, recorder.Code)

	// Unknown orgs are not found.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET",
		pathspec.AsClientPath()+"?org_id=O999", nil))
	assert.Equal(self.T(), http.StatusNotFound, recorder.Code)
}

func (self *DownloadPublicTestSuite) TestDownloadRootFormUpload() {
	path_manager := paths.NewFormUploadPathManager(self.ConfigObj, "rules.yar")
	pathspec, file_store_factory, err := path_manager.Path()
	assert.NoError(self.T(), err)

	writer, err := file_store_factory.WriteFile(pathspec)
	assert.NoError(self.T(), err)

	_, err = writer.Write([]byte("rule Y { condition: true }"))
	assert.NoError(self.T(), err)
	assert.NoError(self.T(), writer.Close())

	handler := downloadPublic(self.ConfigObj, [][]string{{"public"}})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET",
		path_manager.URL(), nil))
	assert.Equal(self.T(), http.StatusOK, recorder.Code)
	assert.Equal(self.T(), "rule Y { condition: true }", recorder.Body.String())
}

func TestDownloadPublic(t *testing.T) {
	suite.Run(t, &DownloadPublicTestSuite{})
}
