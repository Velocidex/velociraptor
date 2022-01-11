package filesystem

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	_ "www.velocidex.com/golang/velociraptor/vql/common"
)

type ZipTestSuite struct {
	test_utils.TestSuite
}

// Make sure that reference counting works well
func (self *ZipTestSuite) TestReferenceCount() {
	zip_file, _ := filepath.Abs("../../artifacts/testdata/files/hello.zip")
	zip_file_pathspec := glob.PathSpec{
		DelegateAccessor: "file",
		DelegatePath:     zip_file,
		Path:             "/**",
	}
	snapshot := vtesting.GetMetrics(self.T(), "accessor_zip_")

	rows, err := test_utils.RunQuery(self.ConfigObj, `
SELECT pathspec(parse=FullPath).Path AS Base,
    read_file(filename=FullPath, length=10, accessor='zip') AS Data
FROM glob(globs=Glob, accessor='zip')
WHERE NOT IsDir`, ordereddict.NewDict().
		Set("Glob", zip_file_pathspec.String()))
	assert.NoError(self.T(), err)

	state := vtesting.GetMetricsDifference(self.T(), "accessor_zip_", snapshot)

	// Zip file must be closed now
	value, _ := state.GetInt64("accessor_zip_current_open")
	assert.Equal(self.T(), int64(0), value)
	value, _ = state.GetInt64("accessor_zip_current_references")
	assert.Equal(self.T(), int64(0), value)

	// We opened the zip file exactly once.
	value, _ = state.GetInt64("accessor_zip_total_open")
	assert.Equal(self.T(), int64(1), value)

	goldie.Assert(self.T(), "TestReferenceCount", json.MustMarshalIndent(rows))
}

// Make sure that reference counting works well
func (self *ZipTestSuite) TestReferenceCountNested() {
	zip_file, _ := filepath.Abs("../../artifacts/testdata/files/hello.zip")
	zip_file_pathspec := glob.PathSpec{
		DelegateAccessor: "file",
		DelegatePath:     zip_file,
		Path:             "/**",
	}
	snapshot := vtesting.GetMetrics(self.T(), "accessor_zip_")

	rows, err := test_utils.RunQuery(self.ConfigObj, `
SELECT * FROM foreach(
row={
  SELECT pathspec(parse=FullPath).Path AS Base,
    read_file(filename=FullPath, length=10, accessor='zip') AS Data
  FROM glob(globs=Glob, accessor='zip')
  WHERE NOT IsDir
}, query={
  SELECT pathspec(parse=FullPath).Path AS Base,
    read_file(filename=FullPath, length=10, accessor='zip') AS Data
  FROM glob(globs=Glob, accessor='zip')
  WHERE NOT IsDir
})`, ordereddict.NewDict().
		Set("Glob", zip_file_pathspec))
	assert.NoError(self.T(), err)

	state := vtesting.GetMetricsDifference(self.T(), "accessor_zip_", snapshot)

	// Zip file must be closed now
	value, _ := state.GetInt64("accessor_zip_current_open")
	assert.Equal(self.T(), int64(0), value)

	value, _ = state.GetInt64("accessor_zip_current_references")
	assert.Equal(self.T(), int64(0), value)

	// We opened the zip file exactly once.
	value, _ = state.GetInt64("accessor_zip_total_open")
	assert.Equal(self.T(), int64(1), value)

	goldie.Assert(self.T(), "TestReferenceCountNested", json.MustMarshalIndent(rows))
}

// Zip files are cached in the root scope so they can be reused across
// local scopes. This test calls the chain() plugin to open the same
// nested zip file in inside local chain scope 10 times. However,
// since the zip files are cached they will only be opened once.
func (self *ZipTestSuite) TestCachedZip() {
	// Read nested ZIP files - the nested.zip contains another zip
	// file, hello.zip which in turn contains some txt files.
	zip_file, _ := filepath.Abs("../../artifacts/testdata/files/nested.zip")
	zip_file_pathspec := glob.PathSpec{
		DelegateAccessor: "zip",
		Delegate: &glob.PathSpec{
			DelegateAccessor: "file",
			DelegatePath:     zip_file,
			Path:             "hello.zip",
		},
		Path: "hello1.txt",
	}

	snapshot := vtesting.GetMetrics(self.T(), "accessor_zip_")

	// Read some non existant files to check that we close everything
	// on error paths.
	rows, err := test_utils.RunQuery(self.ConfigObj, `
LET ZIP_FILE_CACHE_SIZE <= 30

SELECT * from chain(
a={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
b={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
c={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
d={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
e={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
f={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
g={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
h={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
i={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() },
j={ SELECT read_file(accessor="zip", filename=PathSpec) AS Data FROM scope() }
)
`, ordereddict.NewDict().
		Set("PathSpec", zip_file_pathspec))
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 10, len(rows))
	for i := 0; i < 9; i++ {
		data, _ := rows[i].Get("Data")
		assert.Equal(self.T(), "hello1\n", data)
	}

	// Make sure we dont have any dangling references
	state := vtesting.GetMetricsDifference(self.T(), "accessor_zip_", snapshot)

	// Scope is closed - no zip handles are leaking.
	value, _ := state.GetInt64("accessor_zip_current_open")
	assert.Equal(self.T(), int64(0), value)

	value, _ = state.GetInt64("accessor_zip_current_references")
	assert.Equal(self.T(), int64(0), value)

	value, _ = state.GetInt64("accessor_zip_current_tmp_conversions")
	assert.Equal(self.T(), int64(0), value)

	// All up we opened two zip files in total since zip files were
	// cached..
	value, _ = state.GetInt64("accessor_zip_total_open")
	assert.Equal(self.T(), int64(2), value)

	// Make sure we converted one file to tmp file.
	value, _ = state.GetInt64("accessor_zip_total_tmp_conversions")
	assert.Equal(self.T(), int64(1), value)
}

func (self *ZipTestSuite) TestCachedZipWithCacheTrim() {
	// Read nested ZIP files - the nested.zip contains another zip
	// file, hello.zip which in turn contains some txt files.
	zip_file, _ := filepath.Abs("../../artifacts/testdata/files/nested.zip")

	env := ordereddict.NewDict()
	for i := 0; i < 11; i++ {
		zip_file_pathspec := glob.PathSpec{
			DelegateAccessor: "zip",
			Delegate: &glob.PathSpec{
				DelegateAccessor: "file",
				DelegatePath:     zip_file,
				Path:             fmt.Sprintf("hello%d.zip", i),
			},
			Path: "hello1.txt",
		}
		env.Set(fmt.Sprintf("PathSpec%d", i), zip_file_pathspec)
	}

	snapshot := vtesting.GetMetrics(self.T(), "accessor_zip_")

	// Read some non existant files to check that we close everything
	// on error paths. Make the zip cache size very small to ensure we
	// close all files as we go along..
	rows, err := test_utils.RunQuery(self.ConfigObj, `
LET ZIP_FILE_CACHE_SIZE <= 3
SELECT * from chain(
async=TRUE,
a={ SELECT read_file(accessor="zip", filename=PathSpec1) AS Data FROM scope() },
b={ SELECT read_file(accessor="zip", filename=PathSpec2) AS Data FROM scope() },
c={ SELECT read_file(accessor="zip", filename=PathSpec3) AS Data FROM scope() },
d={ SELECT read_file(accessor="zip", filename=PathSpec4) AS Data FROM scope() },
e={ SELECT read_file(accessor="zip", filename=PathSpec5) AS Data FROM scope() },
f={ SELECT read_file(accessor="zip", filename=PathSpec6) AS Data FROM scope() },
g={ SELECT read_file(accessor="zip", filename=PathSpec7) AS Data FROM scope() },
h={ SELECT read_file(accessor="zip", filename=PathSpec8) AS Data FROM scope() },
i={ SELECT read_file(accessor="zip", filename=PathSpec9) AS Data FROM scope() },
j={ SELECT read_file(accessor="zip", filename=PathSpec10) AS Data FROM scope() }
)
`, env)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 10, len(rows))
	for i := 0; i < 9; i++ {
		data, _ := rows[i].Get("Data")
		assert.Equal(self.T(), "hello1\n", data)
	}

	// Make sure we dont have any dangling references
	state := vtesting.GetMetricsDifference(self.T(), "accessor_zip_", snapshot)

	// Scope is closed - no zip handles are leaking.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		state := vtesting.GetMetricsDifference(self.T(), "accessor_zip_", snapshot)
		value, _ := state.GetInt64("accessor_zip_current_open")
		return int64(0) == value
	})

	value, _ := state.GetInt64("accessor_zip_current_references")
	assert.Equal(self.T(), int64(0), value)

	value, _ = state.GetInt64("accessor_zip_current_tmp_conversions")
	assert.Equal(self.T(), int64(0), value)

	// All up we opened 11 zip files in total (the primary one and
	// each embedded zip file
	value, _ = state.GetInt64("accessor_zip_total_open")
	assert.Equal(self.T(), int64(11), value)

	// Each nested zip file was extracted to tmpfile.
	value, _ = state.GetInt64("accessor_zip_total_tmp_conversions")
	assert.Equal(self.T(), int64(10), value)
}

func TestZipAccessor(t *testing.T) {
	suite.Run(t, &ZipTestSuite{})
}
