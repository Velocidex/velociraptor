package api

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type FormUploadTestSuite struct {
	test_utils.TestSuite
	dir string
}

func (self *FormUploadTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()

	var err error
	self.dir, err = tempfile.TempDir("form_upload_test")
	assert.NoError(self.T(), err)

	file_store.ClearGlobalFilestore()

	self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	self.ConfigObj.Datastore.FilestoreDirectory = self.dir
	self.ConfigObj.Datastore.Location = self.dir

	self.TestSuite.SetupTest()
}

func (self *FormUploadTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	file_store.ClearGlobalFilestore()

	_ = os.RemoveAll(self.dir)
}

func (self *FormUploadTestSuite) TestFormUploadToOrgFilestore() {
	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	_, err = org_manager.CreateNewOrg("Test Org", "O123", services.RandomNonce)
	assert.NoError(self.T(), err)

	org_config_obj, err := org_manager.GetOrgConfig("O123")
	assert.NoError(self.T(), err)

	err = services.GrantRoles(org_config_obj, "bob", []string{"investigator"})
	assert.NoError(self.T(), err)

	pack := makeArtifactPack(self.T())
	response := uploadFormFile(self.T(), "O123", "bob", "pack.zip", pack)

	assert.Equal(self.T(), "pack.zip", response.Filename)

	// The upload is readable through the org's filestore the same
	// way LoadArtifactPack reads it.
	zip_reader, closer, err := getZipReader(self.Ctx, org_config_obj,
		&api_proto.LoadArtifactPackRequest{VfsPath: response.VfsPath})
	assert.NoError(self.T(), err)
	defer closer()

	assert.Equal(self.T(), 1, len(zip_reader.File))
	assert.Equal(self.T(), "Custom.Test.yaml", zip_reader.File[0].Name)

	// The upload is not in the root org's filestore.
	pathspec := path_specs.NewUnsafeFilestorePath(response.VfsPath...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)
	_, err = file_store.GetFileStore(self.ConfigObj).ReadFile(pathspec)
	assert.True(self.T(), errors.Is(err, os.ErrNotExist))

	// The URL selects the org so the public handler can serve it.
	upload_url, err := url.Parse(response.Url)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "O123", upload_url.Query().Get("org_id"))
	assert.Equal(self.T(), pathspec.AsClientPath(), upload_url.Path)
}

func (self *FormUploadTestSuite) TestFormUploadToRootFilestore() {
	err := services.GrantRoles(self.ConfigObj, "bob", []string{"investigator"})
	assert.NoError(self.T(), err)

	response := uploadFormFile(self.T(), "root", "bob", "rules.yar",
		[]byte("rule X { condition: true }"))

	pathspec := path_specs.NewUnsafeFilestorePath(response.VfsPath...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)
	data, err := readAll(file_store.GetFileStore(self.ConfigObj), pathspec)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "rule X { condition: true }", string(data))

	upload_url, err := url.Parse(response.Url)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "", upload_url.RawQuery)
}

func TestFormUpload(t *testing.T) {
	suite.Run(t, &FormUploadTestSuite{})
}

func uploadFormFile(t *testing.T, org_id, username, filename string,
	data []byte) *api_proto.FormUploadMetadata {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	err := writer.WriteField("_params_", "{}")
	assert.NoError(t, err)

	part, err := writer.CreateFormFile("file", filename)
	assert.NoError(t, err)

	_, err = part.Write(data)
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())

	user_info, err := json.Marshal(&api_proto.VelociraptorUser{Name: username})
	assert.NoError(t, err)

	ctx := context.WithValue(context.Background(),
		constants.GRPC_USER_CONTEXT, string(user_info))

	req := httptest.NewRequest("POST", "/api/v1/UploadFormFile", body).
		WithContext(ctx)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Grpc-Metadata-Orgid", org_id)

	recorder := httptest.NewRecorder()
	formUploadHandler().ServeHTTP(recorder, req)
	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	response := &api_proto.FormUploadMetadata{}
	err = json.Unmarshal(recorder.Body.Bytes(), response)
	assert.NoError(t, err)

	return response
}

func makeArtifactPack(t *testing.T) []byte {
	buffer := &bytes.Buffer{}
	zip_writer := zip.NewWriter(buffer)

	fd, err := zip_writer.Create("Custom.Test.yaml")
	assert.NoError(t, err)

	_, err = fd.Write([]byte("name: Custom.Test\n"))
	assert.NoError(t, err)
	assert.NoError(t, zip_writer.Close())

	return buffer.Bytes()
}

func readAll(file_store_factory api.FileStore,
	pathspec api.FSPathSpec) ([]byte, error) {
	fd, err := file_store_factory.ReadFile(pathspec)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	result := &bytes.Buffer{}
	_, err = result.ReadFrom(fd)
	return result.Bytes(), err
}
