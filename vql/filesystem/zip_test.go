package filesystem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type ZipTestSuite struct {
	test_utils.TestSuite
}

// Make sure that reference counting works well
func (self *ZipTestSuite) TestReferenceCount() {
	zip_file, _ := filepath.Abs("../../artifacts/testdata/files/hello.zip")

	total_opened := getZipAccessorTotalOpened(self.T())

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict().
			Set("Glob", "file://"+zip_file+"#/**"),
	}
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)

	ctx := context.Background()
	query := `SELECT basename(path=FullPath) AS Base,
    read_file(filename=FullPath, length=10, accessor='zip') AS Data
FROM glob(globs=Glob, accessor='zip')
WHERE NOT IsDir
`

	vql, err := vfilter.Parse(query)
	assert.NoError(self.T(), err)

	rows := []types.Row{}
	for row := range vql.Eval(ctx, scope) {
		rows = append(rows, row)
	}
	scope.Close()

	// Zip file must be closed now
	assert.Equal(self.T(), uint64(0), getZipAccessorCurrentOpened(self.T()))
	assert.Equal(self.T(), uint64(0), getZipAccessorCurrentReferences(self.T()))

	// We opened the zip file exactly once.
	assert.Equal(self.T(), uint64(1),
		getZipAccessorTotalOpened(self.T())-total_opened)

	goldie.Assert(self.T(), "TestReferenceCount", json.MustMarshalIndent(rows))
}

// Make sure that reference counting works well
func (self *ZipTestSuite) TestReferenceCountNested() {
	zip_file, _ := filepath.Abs("../../artifacts/testdata/files/hello.zip")

	total_opened := getZipAccessorTotalOpened(self.T())

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict().
			Set("Glob", zip_file+"#/**"),
	}
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)

	ctx := context.Background()
	query := `
SELECT * FROM foreach(
row={
  SELECT basename(path=FullPath) AS Base,
    read_file(filename=FullPath, length=10, accessor='zip') AS Data
  FROM glob(globs=Glob, accessor='zip')
  WHERE NOT IsDir
}, query={
  SELECT basename(path=FullPath) AS Base,
    read_file(filename=FullPath, length=10, accessor='zip') AS Data
  FROM glob(globs=Glob, accessor='zip')
  WHERE NOT IsDir
})
`

	vql, err := vfilter.Parse(query)
	assert.NoError(self.T(), err)

	rows := []types.Row{}
	for row := range vql.Eval(ctx, scope) {
		rows = append(rows, row)
	}
	scope.Close()

	// Zip file must be closed now
	assert.Equal(self.T(), uint64(0), getZipAccessorCurrentOpened(self.T()))
	assert.Equal(self.T(), uint64(0), getZipAccessorCurrentReferences(self.T()))

	// We opened the zip file exactly once.
	assert.Equal(self.T(), uint64(1),
		getZipAccessorTotalOpened(self.T())-total_opened)

	goldie.Assert(self.T(), "TestReferenceCountNested", json.MustMarshalIndent(rows))
}

func getZipAccessorCurrentOpened(t *testing.T) uint64 {
	gathering, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)

	for _, metric := range gathering {
		if *metric.Name == "accessor_zip_current_open" {
			for _, m := range metric.Metric {
				return uint64(*m.Gauge.Value)
			}
		}
	}
	return 0
}

func getZipAccessorTotalOpened(t *testing.T) uint64 {
	gathering, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)

	for _, metric := range gathering {
		if *metric.Name == "accessor_zip_total_open" {
			for _, m := range metric.Metric {
				return uint64(*m.Counter.Value)
			}
		}
	}
	return 0
}

func getZipAccessorCurrentReferences(t *testing.T) uint64 {
	gathering, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)

	for _, metric := range gathering {
		if *metric.Name == "accessor_zip_current_references" {
			for _, m := range metric.Metric {
				return uint64(*m.Gauge.Value)
			}
		}
	}
	return 0
}

func TestZipAccessor(t *testing.T) {
	suite.Run(t, &ZipTestSuite{})
}
