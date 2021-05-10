package csv

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter/types"
)

type CSVParserTestSuite struct {
	suite.Suite
}

type testCase struct {
	description string
	csv         string
	AutoHeaders bool
	separator   string
}

var csvTestCases = []testCase{
	{
		description: "CSV with column header and JSON",
		csv: `ColA,ColB,ColC
1,2,3
"1","{""a"":1}","[1,2,3]"
`},
	{
		description: "CSV with no headers and linebreak",
		csv: `1,3,4
7,"4
3",5
`,
		AutoHeaders: true},
	{
		description: "CSV with separator",
		csv: `ColA|ColB|ColC
1|2|3
`,
		separator: "|"},
}

func (self *CSVParserTestSuite) TestCSVParser() {
	result := ordereddict.NewDict()
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()

	plugin := ParseCSVPlugin{}

	for idx, test_case := range csvTestCases {
		rows := []types.Row{}
		args := ordereddict.NewDict().
			Set("filename", test_case.csv).
			Set("accessor", "data")

		if test_case.AutoHeaders {
			args.Set("auto_headers", test_case.AutoHeaders)
		}

		if test_case.separator != "" {
			args.Set("separator", test_case.separator)
		}

		for row := range plugin.Call(ctx, scope, args) {
			rows = append(rows, row)
		}
		result.Set(fmt.Sprintf("%v: %v", idx, test_case.description), rows)
	}
	goldie.Assert(self.T(), "TestCSVParser", json.MustMarshalIndent(result))
}

func TestCSVParser(t *testing.T) {
	suite.Run(t, &CSVParserTestSuite{})
}
