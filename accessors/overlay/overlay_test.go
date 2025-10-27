package overlay

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

type OverlayAccessorTestSuite struct {
	suite.Suite
	tmpdir string
}

func (self *OverlayAccessorTestSuite) SetupTest() {
	tmpdir, err := tempfile.TempDir("accessor_test")
	assert.NoError(self.T(), err)

	self.tmpdir = strings.ReplaceAll(tmpdir, "\\", "/")
}

func (self *OverlayAccessorTestSuite) TearDownTest() {
	os.RemoveAll(self.tmpdir) // clean up
}

func (self *OverlayAccessorTestSuite) makeFile(path string, content string) {
	path = strings.TrimLeft(path, "/")

	file_path := filepath.Join(self.tmpdir, path)
	os.MkdirAll(filepath.Dir(file_path), 0700)

	fd, err := os.OpenFile(file_path,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	assert.NoError(self.T(), err)
	fd.Write([]byte(content))
	fd.Close()
}

func (self *OverlayAccessorTestSuite) TestOverlay() {
	self.makeFile("foo1/file1.txt", "Hello")
	self.makeFile("foo2/file2.txt", "Hello Two")

	scope := vql_subsystem.MakeScope().
		AppendVars(ordereddict.NewDict().
			Set(vql_subsystem.ACL_MANAGER_VAR,
				acl_managers.NullACLManager{}).
			Set(constants.OVERLAY_ACCESSOR_DELEGATES,
				ordereddict.NewDict().
					Set("accessor", "file").
					Set("paths", []string{
						self.tmpdir + "/foo1",
						self.tmpdir + "/foo2",
					})))

	scope.SetLogger(log.New(os.Stderr, " ", 0))
	accessor, err := accessors.GetAccessor("overlay", scope)
	assert.NoError(self.T(), err)

	files, err := accessor.ReadDir("/")
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict()

	for _, f := range files {
		fd, err := accessor.OpenWithOSPath(f.OSPath())
		assert.NoError(self.T(), err)

		data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
		assert.NoError(self.T(), err)
		fd.Close()

		golden.Set(f.OSPath().String(), string(data))
	}

	goldie.Assert(self.T(), "TestOverlay", json.MustMarshalIndent(golden))

}

// Test both the Windows and Linux File accessor.
func TestOverlayAccessor(t *testing.T) {
	suite.Run(t, &OverlayAccessorTestSuite{})
}
