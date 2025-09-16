package tools

import (
	"crypto/rand"
	"encoding/hex"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/server"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

var (
	msi_src          = ""
	binary_src       = ""
	repacked_msi_dst = ""
	repacked_dst     = ""

/*
// Set these to capture the repacked file for manual inspection.
repacked_msi_dst = "/tmp/m_repacked.msi"
repacked_dst     = "/tmp/m_repacked.exe"

// Provide the real binary and msi so they can be packed then
// inspect the produced data
binary_src = "../../output/velociraptor.exe"
msi_src    = "/tmp/velociraptor-v0.6.8-rc1-windows-amd64.msi"
*/
)

type RepackTestSuite struct {
	test_utils.TestSuite
}

func (self *RepackTestSuite) TestRepackBinary() {
	ctx := self.Ctx

	dir, err := tempfile.TempDir("tmp")
	assert.NoError(self.T(), err)

	defer os.RemoveAll(dir)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Uploader: &uploads.FileBasedUploader{
			UploadDir: dir,
		},
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()

	if binary_src == "" {
		binary_src = filepath.Join(dir, "binary.exe")
		fd, err := os.OpenFile(binary_src, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
		assert.NoError(self.T(), err)

		fd.Write(config.FileConfigDefaultYaml)
		fd.Close()
	} else {
		binary_src, _ = filepath.Abs(binary_src)
	}

	accessor, err := accessors.GetAccessor("file", scope)
	assert.NoError(self.T(), err)

	tool_pathspec, err := accessor.ParsePath(binary_src)
	assert.NoError(self.T(), err)

	// Add the Windows Binary to the inventory
	result := (&server.InventoryAddFunction{}).Call(
		ctx, scope, ordereddict.NewDict().
			Set("tool", "VelociraptorWindows").
			Set("accessor", "file").
			Set("file", tool_pathspec))

	_, ok := result.(*ordereddict.Dict)
	assert.True(self.T(), ok, "Result type is %T", result)

	result = RepackFunction{}.Call(
		ctx, scope,
		ordereddict.NewDict().
			// Set a fixed client id to keep it predictable
			Set("target", "VelociraptorWindows").
			Set("config", `
autoexec:
 argv:
  - help
`).
			Set("binaries", []string{"VelociraptorWindows"}).
			Set("upload_name", "test.zip"))

	upload_response, ok := result.(*ordereddict.Dict)
	assert.True(self.T(), ok, "Result type is %T", result)

	upload_response_path, _ := upload_response.GetString("Path")

	// Save a copy of the repacked data for inspection.
	if repacked_dst != "" {
		utils.CopyFile(ctx, upload_response_path, repacked_dst, 0644)
		scope.Log("Stored repacked binary in %v for manual inspection", repacked_dst)
	}

	// Check the content of the packed binaries.
	fd, err := os.Open(upload_response_path)
	assert.NoError(self.T(), err)
	s, err := fd.Stat()
	assert.NoError(self.T(), err)

	zip, err := zip.NewReader(fd, s.Size())
	assert.NoError(self.T(), err)

	files := []string{}
	for _, f := range zip.File {
		files = append(files, f.Name)
	}
	assert.Equal(self.T(), []string{
		"uploads/binary.exe",
		"uploads/inventory.csv",
	}, files)
}

// The Generic Embedded container allows packing arbitrary sized
// payloads. Unlike repacking the executable itself, the amount of
// space available is unlimited as we can just grow the file as much
// as needed. This is a valid workaround for when we need to repack
// very large payloads into the offline collector which are too large
// to fit in the pre-allocated space.
func (self *RepackTestSuite) TestRepackGenericContainer() {
	ctx := self.Ctx

	dir, err := tempfile.TempDir("tmp")
	assert.NoError(self.T(), err)

	defer os.RemoveAll(dir)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Uploader: &uploads.FileBasedUploader{
			UploadDir: dir,
		},
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()

	binary_src = filepath.Join(dir, "offline.sh")
	fd, err := os.OpenFile(binary_src, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
	assert.NoError(self.T(), err)

	fd.Write([]byte(`#!/bin/sh

###<Begin Embedded Config>
#############################################################################
`))
	fd.Close()

	accessor, err := accessors.GetAccessor("file", scope)
	assert.NoError(self.T(), err)

	tool_pathspec, err := accessor.ParsePath(binary_src)
	assert.NoError(self.T(), err)

	// Add the Windows Binary to the inventory
	result := (&server.InventoryAddFunction{}).Call(
		ctx, scope, ordereddict.NewDict().
			Set("tool", "GenericCollector").
			Set("accessor", "file").
			Set("file", tool_pathspec))

	_, ok := result.(*ordereddict.Dict)
	assert.True(self.T(), ok, "Result type is %T", result)

	// Make the embedded config really large. This is ok for the
	// Generic container because we just grow it as needed.
	embedded_config := `
autoexec:
 argv:
  - help
 artifact_definitions:
  - name: CustomArtifact
    description: |
`
	// Make some random data
	data := make([]byte, 200*1024)
	rand.Read(data)
	data_str := hex.EncodeToString(data)

	for i := 0; i < len(data_str)-80; i += 80 {
		embedded_config += "\n     " + data_str[i:i+80]
	}

	result = RepackFunction{}.Call(
		ctx, scope,
		ordereddict.NewDict().
			// Set a fixed client id to keep it predictable
			Set("target", "GenericCollector").
			Set("config", embedded_config).
			Set("binaries", []string{"GenericCollector"}).
			Set("upload_name", "test.zip"))

	upload_response, ok := result.(*ordereddict.Dict)
	assert.True(self.T(), ok, "Result type is %T", result)

	upload_response_path, _ := upload_response.GetString("Path")

	// Now extract the config from the result.
	config_obj, err := config.ExtractEmbeddedConfig(upload_response_path)
	assert.NoError(self.T(), err)

	// Make sure all the encoded config is preserved
	encoded_data := strings.ReplaceAll(
		config_obj.Autoexec.ArtifactDefinitions[0].Description,
		"\n", "")
	assert.Equal(self.T(),
		encoded_data, data_str[:len(encoded_data)])

	// Save a copy of the repacked data for inspection.
	if repacked_dst != "" {
		utils.CopyFile(ctx, upload_response_path, repacked_dst, 0644)
		scope.Log("Stored repacked binary in %v for manual inspection", repacked_dst)
	}

	// Check the content of the packed binaries.
	fd, err = os.Open(upload_response_path)
	assert.NoError(self.T(), err)
	s, err := fd.Stat()
	assert.NoError(self.T(), err)

	zip, err := zip.NewReader(fd, s.Size())
	assert.NoError(self.T(), err)

	files := []string{}
	for _, f := range zip.File {
		files = append(files, f.Name)
	}
	assert.Equal(self.T(), []string{
		"uploads/offline.sh",
		"uploads/inventory.csv",
	}, files)
}

func (self *RepackTestSuite) TestRepackMSI() {
	ctx := self.Ctx

	dir, err := tempfile.TempDir("tmp")
	assert.NoError(self.T(), err)

	defer os.RemoveAll(dir)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Uploader: &uploads.FileBasedUploader{
			UploadDir: dir,
		},
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()

	// Build a file that looks like an msi for testing.
	if msi_src == "" {
		msi_src = filepath.Join(dir, "binary.msi")
		fd, err := os.OpenFile(msi_src, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
		assert.NoError(self.T(), err)

		fd.Write([]byte(MSI_MAGIC))
		template, err := os.Open("../../docs/wix/output/client.config.yaml")
		assert.NoError(self.T(), err)

		data, err := ioutil.ReadAll(template)
		assert.NoError(self.T(), err)

		fd.Write(data)
		fd.Close()
	} else {
		msi_src, _ = filepath.Abs(msi_src)
	}

	accessor, err := accessors.GetAccessor("file", scope)
	assert.NoError(self.T(), err)

	tool_pathspec, err := accessor.ParsePath(msi_src)
	assert.NoError(self.T(), err)

	// Add the Windows Binary to the inventory
	result := (&server.InventoryAddFunction{}).Call(
		ctx, scope, ordereddict.NewDict().
			Set("tool", "VelociraptorWindows").
			Set("accessor", "file").
			Set("file", tool_pathspec))

	_, ok := result.(*ordereddict.Dict)
	assert.True(self.T(), ok, "Result type is %T", result)

	result = RepackFunction{}.Call(
		ctx, scope,
		ordereddict.NewDict().
			// Set a fixed client id to keep it predictable
			Set("target", "VelociraptorWindows").
			Set("config", `
autoexec:
 argv:
  - help
`).
			Set("upload_name", "test.zip"))

	upload_response, ok := result.(*ordereddict.Dict)
	assert.True(self.T(), ok, "Result type is %T", result)

	upload_response_path, _ := upload_response.GetString("Path")

	// Save a copy of the repacked data for inspection.
	if repacked_dst != "" {
		utils.CopyFile(ctx, upload_response_path, repacked_msi_dst, 0644)
		scope.Log("Stored repacked msi in %v for manual inspection", repacked_msi_dst)
	}
}

func TestRepackPlugin(t *testing.T) {
	suite.Run(t, &RepackTestSuite{})
}
