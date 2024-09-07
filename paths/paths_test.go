package paths_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type PathManagerTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
}

func (self *PathManagerTestSuite) SetupTest() {
	self.config_obj = config.GetDefaultConfig()
	self.config_obj.Datastore.Location = "/ds"
	self.config_obj.Datastore.FilestoreDirectory = "/fs"
}

func (self *PathManagerTestSuite) TestAsClientPath() {
	path_spec := path_specs.NewSafeFilestorePath("a", "b", "c").
		SetType(api.PATH_TYPE_FILESTORE_JSON)

	client_path := path_spec.AsClientPath()

	// The client path needs to carry the extension
	assert.True(self.T(), strings.HasSuffix(client_path, ".json"))

	// Parse the path back into a path spec - this should restore
	// the type from the extension.
	new_path_spec := ExtractClientPathSpec("", client_path)

	assert.Equal(self.T(), new_path_spec.Type(), path_spec.Type())
	assert.Equal(self.T(), new_path_spec.Components(), path_spec.Components())
}

// Use the path spec to store to the data store and figure out exactly
// where the file will be created. Return this path - this includes
// any file store escaping or path transformations.
func (self *PathManagerTestSuite) getDatastorePath(path_spec api.DSPathSpec) string {
	ds := datastore.NewMemcacheDataStore(context.Background(), self.config_obj)
	data := &crypto_proto.VeloMessage{}
	ds.SetSubject(self.config_obj, path_spec, data)

	results := []string{}
	for _, k := range ds.Dump() {
		if k.IsDir() {
			continue
		}
		results = append(results, normalize_path(
			datastore.AsDatastoreFilename(ds, self.config_obj, k)))
	}
	assert.Equal(self.T(), 1, len(results))

	// Check that ListChildren() returns the correct path.
	children, err := ds.ListChildren(self.config_obj, path_spec.Dir())
	assert.NoError(self.T(), err)

	for _, child := range children {
		assert.Equal(self.T(),
			child.AsClientPath(),
			path_spec.AsClientPath())
	}

	return results[0]
}

func normalize_path(filename string) string {
	return strings.ReplaceAll(strings.TrimLeft(filename, "\\?"), "\\", "/")
}

// Gets the actual file store path written (including escapes)
func (self *PathManagerTestSuite) getFilestorePath(path_spec api.FSPathSpec) string {
	fs := memory.NewMemoryFileStore(self.config_obj)
	fs.Clear()

	fd, err := fs.WriteFile(path_spec)
	assert.NoError(self.T(), err)

	fd.Write([]byte(""))
	fd.Close()

	results := []string{}
	for _, k := range fs.Data.Keys() {
		results = append(results, k)
	}
	assert.Equal(self.T(), 1, len(results))

	return results[0]
}

func TestPathManagers(t *testing.T) {
	suite.Run(t, &PathManagerTestSuite{})
}

// Breaks a client path into components. The client's path may consist
// of a drive letter or a device which will be treated as a single
// component. For example:
// C:\Windows -> "C:\", "Windows"
// \\.\c:\Windows -> "\\.\C:", "Windows"

// Other components that contain path separators need to be properly
// quoted as usual:
// HKEY_LOCAL_MACHINE\Software\Microsoft\"http://www.google.com"\Foo ->
// "HKEY_LOCAL_MACHINE", "Software", "Microsoft", "http://www.google.com", "Foo"
func ExtractClientPathSpec(accessor, path string) api.FSPathSpec {
	result := path_specs.NewUnsafeFilestorePath()
	if accessor != "" {
		result = result.AddChild(accessor)
	}

	components := paths.ExtractClientPathComponents(path)

	// Restore the PathSpec type from its extensions
	if len(components) > 0 {
		last := len(components) - 1
		name_type, name := api.GetFileStorePathTypeFromExtension(
			components[last])
		components[last] = name
		result = result.SetType(name_type)
	}

	return result.AddChild(components...)
}
