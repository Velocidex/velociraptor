package parsers

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter/types"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

type RegexParserTestSuite struct {
	suite.Suite
}

type testCase struct {
	description          string
	data                 string
	First_row_is_headers bool
	Columns              []string
	Regex                []string
	RecordRegex          string
	BufferSize           int
}

var splitTestCases = []testCase{
	{
		description:          "CSV Like split cases",
		First_row_is_headers: true,
		Regex:                []string{","},
		data: `ColA,ColB,ColC
1,2,3
Foo,Bar,Baz
`},
	{
		description:          "CSV Like split cases with small buffer (at least size of line)",
		First_row_is_headers: true,
		Regex:                []string{","},
		BufferSize:           18,
		data: `ColA,ColB,ColC
1,2,3
Foo,Bar,Baz
`},
	{
		description: "Empty lines",
		data: `
A
B

C
`},
	{
		description:          "Empty lines with two columns",
		First_row_is_headers: true,
		data: `Col1 Col2
b  d

c  s
`},
	{
		description:          "Record Separators",
		First_row_is_headers: true,
		RecordRegex:          `\|`,
		data:                 `Col1 Col2|b  d|c  s`},
}

func (self *RegexParserTestSuite) TestSplitRecordParser() {
	result := ordereddict.NewDict()
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()

	plugin := SplitRecordParser{}

	for idx, test_case := range splitTestCases {
		regex := ""
		if len(test_case.Regex) > 0 {
			regex = test_case.Regex[0]
		}

		rows := []types.Row{}
		args := ordereddict.NewDict().
			Set("filenames", test_case.data).
			Set("accessor", "data").
			Set("regex", regex).
			Set("record_regex", test_case.RecordRegex).
			Set("buffer_size", test_case.BufferSize).
			Set("first_row_is_headers", test_case.First_row_is_headers)

		for row := range plugin.Call(ctx, scope, args) {
			rows = append(rows, row)
		}
		result.Set(fmt.Sprintf("%v: %v", idx, test_case.description), rows)
	}
	goldie.Assert(self.T(), "TestSplitRecordParser", json.MustMarshalIndent(result))
}

// parse_records_with_regex() is intended to pick out records from the
// data and pass them for further processing (usually
// parse_string_with_regex()). This is suitable for files where data
// is stored in a semi-structured format: well structured records are
// appended back to back with a delimiter.
var regexTestCases = []testCase{
	{
		description:          "Records are split with delimiter",
		First_row_is_headers: true,
		Regex:                []string{"(?ms)^(?P<Record>.+?)Delimiter"},

		// In this data the Delimiter comes after the record and
		// before the next record
		data: `Record 1 ... Delimiter Record 2 ... Delimiter Some padding that should be ignored`,
	},
	{
		description:          "Records are split with delimiter small buffer",
		First_row_is_headers: true,
		Regex:                []string{"(?ms)^(?P<Record>.+?)Delimiter"},
		BufferSize:           25, // should be larger than the record.
		data:                 `Record 1 ... Delimiter Record 2 ... Delimiter Some padding that should be ignored`,
	},
	{
		description:          "Different types of records mixed in",
		First_row_is_headers: true,
		Regex: []string{
			// Will try to extract each record in turn - the first
			// matching will consume the data.
			"(?ms)^ +Special Delimiter(?P<SpecialRecord>.+?)Delimiter",
			"(?ms)(?P<Record>.+?)Delimiter",
		},
		data: `Record 1 ... Delimiter Special Delimiter Record 2 ... Delimiter Some padding that should be ignored`,
	},
}

func (self *RegexParserTestSuite) TestParseRecordsWithRegex() {
	result := ordereddict.NewDict()
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()

	plugin := _ParseFileWithRegex{}

	for idx, test_case := range regexTestCases {
		if idx != 2 {
			continue
		}
		rows := []types.Row{}
		args := ordereddict.NewDict().
			Set("file", test_case.data).
			Set("accessor", "data").
			Set("regex", test_case.Regex).
			Set("buffer_size", test_case.BufferSize)

		for row := range plugin.Call(ctx, scope, args) {
			rows = append(rows, row)
		}
		result.Set(fmt.Sprintf("%v: %v", idx, test_case.description), rows)
	}
	goldie.Assert(self.T(), "TestParseFileWithRegex",
		json.MustMarshalIndent(result))
}

func TestRegexParser(t *testing.T) {
	suite.Run(t, &RegexParserTestSuite{})
}
