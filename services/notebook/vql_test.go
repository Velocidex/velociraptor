package notebook_test

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/vfilter"
)

// Mocks
type GuiTemplateEngineMock struct {
	errors []string
}

func (mock *GuiTemplateEngineMock) Error(fmt_str string, argv ...interface{}) string {
	mock.errors = append(mock.errors, fmt_str)
	return ""
}

func (mock *GuiTemplateEngineMock) Execute(report *artifacts_proto.Report) (string, error) {
	return "[comments]", nil
}

func (mock *GuiTemplateEngineMock) RunQuery(vql *vfilter.VQL, result []*paths.NotebookCellQuery) ([]*paths.NotebookCellQuery, error) {
	return result, nil
}

func (mock *GuiTemplateEngineMock) Table(values ...interface{}) interface{} {
	return "[table data]"
}

type VQLTestSuite struct {
	test_utils.TestSuite
	tmpl GuiTemplateEngineMock
	sut  *notebook.VqlCell
}

func (self *VQLTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.NotebookService = true

	self.TestSuite.SetupTest()

	self.tmpl = GuiTemplateEngineMock{}
	self.sut = &notebook.VqlCell{}
}

// TESTS
func (self *VQLTestSuite) TestNotebookVqlProcessEmptyQuery() {
	result, err := self.sut.Process(&self.tmpl, "")

	expectedErrors := []string{
		"Please specify a query to run",
	}

	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "", result)
	assert.Equal(self.T(), expectedErrors, self.tmpl.errors)
}

func (self *VQLTestSuite) TestNotebookVqlProcessLet() {
	const vql_string string = `
LET Test = "dummy data"

SELECT Test AS Test1, Test AS Test2, Test AS Test3
FROM range(start=0, end=10, step=1)
`
	const expected string = "[table data]"

	result, err := self.sut.Process(&self.tmpl, vql_string)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), expected, result)
}

func (self *VQLTestSuite) TestNotebookVqlProcessBadVql() {
	const vql_string string = `
// just comments, and bad VQL
NOT VQL
`
	const expected string = ""

	result, err := self.sut.Process(&self.tmpl, vql_string)
	assert.Error(self.T(), err)
	assert.Equal(self.T(), expected, result)
}

func (self *VQLTestSuite) TestNotebookVqlProcessVqlWithComments() {
	const vql_string string = `
/* 
	multiline comments
	line 2
*/
SELECT * from info()
`
	const expected string = "[comments][table data]"

	result, err := self.sut.Process(&self.tmpl, vql_string)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), expected, result)
}

func (self *VQLTestSuite) TestNotebookVqlProcessMultipleVqls() {
	const vql_string string = `
SELECT * from info()
SELECT * from something()
SELECT * from somethingElse()
`
	const expected string = "[table data][table data][table data]"

	result, err := self.sut.Process(&self.tmpl, vql_string)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), expected, result)
}

func (self *VQLTestSuite) TestNotebookVqlProcessMultipleVqlsAndComments() {
	const vql_string string = `
/* 
	multiline comment
*/
SELECT * from info()
/* 
	multiline comment
*/
SELECT * from something()
/* 
	multiline comment
*/
SELECT * from somethingElse()
/* 
	multiline comment
*/
/* 
	multiline comment
*/
SELECT * from somethingElse()
SELECT * from somethingElse()
`
	const expected string = "[comments][table data][comments][table data][comments][table data][comments][table data][table data]"

	result, err := self.sut.Process(&self.tmpl, vql_string)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), expected, result)
}

func (self *VQLTestSuite) TestNotebookVqlProcessOnlyComments() {
	const vql_string string = `
/* 
	multiline comments
	line 2
*/
// Single Line Comment
`
	const expected string = ""

	result, err := self.sut.Process(&self.tmpl, vql_string)
	assert.Error(self.T(), err)
	assert.Equal(self.T(), expected, result)
}

func TestVQL(t *testing.T) {
	suite.Run(t, &VQLTestSuite{})
}
