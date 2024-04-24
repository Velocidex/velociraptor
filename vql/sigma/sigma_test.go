package sigma

import (
	"bytes"
	"context"
	"encoding/base64"
	"log"
	"os"
	"sort"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"

	// For map[string]interface{} protocl
	_ "www.velocidex.com/golang/velociraptor/vql/parsers"
)

type MockQuery struct {
	rows []*ordereddict.Dict
}

func (self *MockQuery) ToString(scope types.Scope) string {
	return "Mock Query"
}

func (self *MockQuery) Eval(ctx context.Context, scope types.Scope) <-chan types.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		for _, r := range self.rows {
			output_chan <- r
		}

	}()

	return output_chan
}

type testCase struct {
	description, rule string
	default_details   string
	fieldmappings     *ordereddict.Dict
	rows              []*ordereddict.Dict
	log_regex         string
	debug             bool
}

var (
	noopRule = `
title: NoopRule
logsource:
   product: windows
   service: application

detection:
  selection:
     EID:
       - 2

  condition: selection
`

	testRows = []*ordereddict.Dict{
		ordereddict.NewDict().
			Set("Foo", "Bar").
			Set("Integer", 4).
			Set("List", []int64{1, 2, 3}).
			Set("Dict", map[string]interface{}{
				"X": 1, "Y": 2,
			}).
			Set("Baz", "Hello"),
		ordereddict.NewDict().
			Set("System", ordereddict.NewDict().
				Set("EventID", 2)),
	}

	sigmaTestCases = []testCase{
		{
			description: "Match single field",
			rule: `
title: SingleField
logsource:
   product: windows
   service: application

detection:
  selection:
     Foo: Bar
  condition: selection
`,
			fieldmappings: ordereddict.NewDict(),
			rows:          testRows,
		},
		{
			description: "Rule With Details",
			rule: `
title: RuleWithDetails
details: This is column Foo=%Foo% Int=%Integer% List=%List% Dict=%Dict%
logsource:
   product: windows
   service: application

detection:
  selection:
     Foo: Bar
     Integer: 4
  condition: selection
`,
			fieldmappings: ordereddict.NewDict(),
			rows:          testRows,
		},
		{
			description: "Default Details in callback",
			rule: `
title: RuleWithDetails
logsource:
   product: windows
   service: application

detection:
  selection:
     Foo: Bar
  condition: selection
`,
			fieldmappings: ordereddict.NewDict(),
			rows:          testRows,
			// Show that the default details call back has access to
			// the scope and returns a string with interpolations.
			default_details: "x=>ScopeVar + 'Default Detail Foo=%Foo%'",
		},
		{
			description: "Match field with regex",
			rule: `
title: RegexField
logsource:
   product: windows
   service: application

# Case insensitive Regex matching
detection:
  selection:
     Foo|re: b.r
  condition: selection
`,
			fieldmappings: ordereddict.NewDict(),
			rows:          testRows,
		},
		{
			description: "Match field with logical operators",
			rule: `
title: AndRule
logsource:
   product: windows
   service: application

# Case insensitive Regex matching
detection:
  selection:
     Foo|re: b.r
  selection2:
     Baz|re: h.+lo

  condition: selection and selection2
`,
			fieldmappings: ordereddict.NewDict(),
			rows:          testRows,
		},
		{
			description: "Match field with logical or operator",
			rule: `
title: OrRule
logsource:
   product: windows
   service: application

# Case insensitive Regex matching
detection:
  selection:
     Foo|re: b.r
  selection2:
     Baz|re: something

  condition: selection or selection2
`,
			fieldmappings: ordereddict.NewDict(),
			rows:          testRows,
		},
		{
			description: "Match simple logsource",
			rule:        noopRule,
			fieldmappings: ordereddict.NewDict().
				Set("EID", "x=>x.System.EventID"),
			rows: testRows,
		},
		{
			description: "Match field with vql operator",
			rule: `
title: VqlRule
logsource:
   product: windows
   service: application

custom_field: Some Custom Field

# VQL modifier can operate on a field or has access to the
# entire rule via the Rule member which also has access
# to custom fields.
detection:
  selection:
     Foo|vql: x=>log(message="Field %v evaluated on Event %v", args=[Rule.AdditionalFields.custom_field, Event])
  condition: selection
`,
			fieldmappings: ordereddict.NewDict(),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Foo", "Bar").
					Set("Baz", "Hello"),
				ordereddict.NewDict().
					Set("Foo", "Baz").
					Set("Baz", "Bye"),
			},
		},
		{
			description: "Match with no condition",
			rule: `
title: NoConditions
logsource:
   product: windows
   service: application

detection:
  selection:
     Foo: bar
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo"),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Foo", "bar").
					Set("Baz", "Hello"),
			},
		},
		{
			description: "Match with NULL",
			rule: `
title: NullRule
logsource:
   product: windows
   service: application

detection:
  selection:
     Foo: null
     Bar: 1
  condition: selection
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo").
				Set("Bar", "x=>x.Bar"),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Bar", 1),
			},
		},
		{
			description: "Unknown modifiers",
			rule: `
title: BadModifiers
logsource:
   product: windows
   service: application

detection:
  selection:
     Foo|somemodifier: XXXX
  condition: selection
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo"),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Foo", "Bar"),
			},
			log_regex: "unknown modifier somemodifier",
		},
		{
			description: "All modifier",
			rule: `
title: BadModifiers
logsource:
   product: windows
   service: application

detection:
  selection:
     Foo|contains|all:
       - a
       - B
  condition: selection
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo"),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Foo", "Bar"),
			},
		},
		{
			description: "Match single base64offset field",
			rule: `
title: Base64 offsets
logsource:
  product: windows
  service: application

detection:
  selection1:
     Foo|base64offset|contains: hello
  selection2:
     Foo|base64offset|contains: test
  selection3:
    Foo|base64offset|contains|all:
      - sprite
      - pepsi
  selection4:
    Foo|base64offset|contains:
      - velo
      - ciraptorex
  condition: (selection1 and selection2) or selection3 or selection4
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo"),
			debug: true,
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Match", "Should match selection1 and selection2 contains single element").
					Set("Decoded", "jejfjefhellorfriufirtestkdkdg").
					Set("Foo", base64.StdEncoding.EncodeToString([]byte("jejfjefhellorfriufirtestkdkdg"))),
				ordereddict.NewDict().
					Set("Match", "Should match selection1 and selection2 contains single element (Shift 1)").
					Set("Decoded", "ejfjefhellorfriufirtestkdkdg").
					Set("Foo", base64.StdEncoding.EncodeToString([]byte("ejfjefhellorfriufirtestkdkdg"))),
				ordereddict.NewDict().
					Set("Match", "Should match selection1 and selection2 contains single element (Shift 2)").
					Set("Decoded", "jfjefhellorfriufirtestkdkdg").
					Set("Foo", base64.StdEncoding.EncodeToString([]byte("jfjefhellorfriufirtestkdkdg"))),
				ordereddict.NewDict().
					Set("Match", "Should match selection4 with contains one of members").
					Set("Decoded", "kgkrgrgveloefjefe").
					Set("Foo", base64.StdEncoding.EncodeToString([]byte("kgkrgrgveloefjefe"))),
				ordereddict.NewDict().
					Set("Match", "Should match selection3 with all").
					Set("Decoded", "kgkrpepsigrgspriteefjefe").
					Set("Foo", base64.StdEncoding.EncodeToString([]byte("kgkrpepsigrgspriteefjefe"))),
			},
		},
	}
)

type SigmaTestSuite struct {
	suite.Suite
}

func (self *SigmaTestSuite) TestSigma() {
	result := ordereddict.NewDict()

	ctx := context.Background()
	scope := vql_subsystem.MakeScope().
		AppendVars(ordereddict.NewDict().Set("ScopeVar", "I'm a scope var:"))

	defer scope.Close()

	plugin := SigmaPlugin{}

	for _, test_case := range sigmaTestCases {
		log_collector := &bytes.Buffer{}
		scope.SetLogger(log.New(log_collector, "", 0))

		rows := []types.Row{}
		args := ordereddict.NewDict().
			Set("rules", test_case.rule).
			Set("log_sources", &LogSourceProvider{
				queries: map[string]types.StoredQuery{
					"*/windows/application": &MockQuery{
						rows: test_case.rows,
					},
				},
			}).
			Set("field_mapping", test_case.fieldmappings)

		if test_case.debug {
			args.Set("debug", true)
		}

		if test_case.default_details != "" {
			args.Set("default_details", test_case.default_details)
		}

		for row := range plugin.Call(ctx, scope, args) {
			rows = append(rows, row)
		}

		sort.Slice(rows, func(i, j int) bool {
			serialized1 := json.MustMarshalString(rows[i])
			serialized2 := json.MustMarshalString(rows[j])
			return string(serialized1) < string(serialized2)
		})

		result.Set(test_case.description, rows)

		if test_case.log_regex != "" {
			assert.Regexp(self.T(), test_case.log_regex,
				string(log_collector.Bytes()))
		}

		os.Stderr.Write(log_collector.Bytes())
	}

	goldie.Assert(self.T(), "TestSigma",
		json.MustMarshalIndent(result))
}

func TestSigmaPlugin(t *testing.T) {
	suite.Run(t, &SigmaTestSuite{})
}
