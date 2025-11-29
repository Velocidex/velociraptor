package sigma

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"

	// For map[string]interface{} protocol
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

	simpleTemporalCorrelationRule = `
title: Rule1
id: r1
logsource:
  product: windows # Just for testing
  service: security
detection:
    selection_parent:
        ParentImage|endswith:
            - '\tomcat8.exe'
    condition: all of selection_*
---
title: Rule2
id: r2
logsource:
  product: windows # Just for testing
  service: security
detection:
    selection_method:
        cs-method: 'POST'
    condition: all of selection_*
---
title: Both rules
correlation:
  type: temporal
  rules:
    - r1
    - r2
  timespan: 10m
level: high
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
			description: "Rule With Details with arrays",
			rule: `
title: RuleWithDetailsWithArrays
# Indexes are 1 based - first element is %Data[1]%
details: This is column Foo=%Data[1]% Bar=%Data[2]% Exceeded=%Data[6]%
logsource:
   product: windows
   service: application

detection:
  selection:
     Foo: Bar
  condition: selection
`,
			fieldmappings: ordereddict.NewDict(),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Foo", "Bar").
					Set("Data", []string{"Element1", "Element2"}),
			},
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
title: All Modifiers
logsource:
   product: windows
   service: application

detection:
  contains_all_true:
     Foo|contains|all:
       - a
       - B

  contains_one_of_true:
     Foo|contains:
       - a
       - C

  contains_no_match_false:
     Foo|contains:
       - Z
       - C

  condition: contains_all_true or contains_one_of_true or contains_no_match_false
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo"),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Foo", "Bar"),
			},
		},

		// Taken from https://sigmahq.io/docs/basics/modifiers.html
		{
			description: "Test Modifiers",
			rule: `
title: Test Modifiers
logsource:
  product: windows
  service: application

detection:
  contains_all:
     uri|contains|all:
        - '/ecp/default.aspx'
        - '__VIEWSTATEGENERATOR='
        - '__VIEWSTATE='

  cidr_1:
     ip_address1|cidr: 192.0.0.0/8

  # Match any of the CIDR
  cidr_2:
     ip_address2|cidr:
         - 192.168.0.0/23
         - 192.168.1.0/23

  contains:
     fieldname|contains: needle

  contains_any:
     fieldname|contains:
        - needle
        - haystack

  contains_all:
     fieldname|contains|all:
        - needle
        - haystack

  startswith:
     fieldname|startswith: needle

  endswith:
     fieldname|endswith: needle

  gt:
     fieldname_int|gt: 15

  gte:
     fieldname_int|gte: 15

  lt:
     fieldname_int|lt: 15

  lte:
     fieldname_int|lte: 15

  re:
     fieldname|re: .*needle$

  # Any regex should match
  re_multiple:
     fieldname|re:
       - ".*needle$"
       - foobar

  # All regex need to match
  re_multiple_all:
     fieldname|re|all:
       - ".*needle$"
       - is a

  wide:
     CommandLineWide|wide|base64offset|contains: "ping"

  wide_any:
     CommandLineWide|wide|base64offset|contains:
        - ping
        - pong

  # Should match all terms
  wide_all:
     CommandLineWide|wide|base64offset|contains|all:
        - "ping"
        - "pong"

  windash:
     CommandLine|windash|contains:
        - " -param-name "
        - " -f "

  windash_bar:
     CommandLine|windash|contains:
        - " -g "

  windash_emdash:
     CommandLine|windash|contains:
        - " -h "

  windash_all:
     CommandLine|windash|contains|all:
        - " -param-name "
        - " -f "

  # An all modifier without a field name
  naked_all:
     "|all":
       - ping
       - ip_address2

  condition: contains
`,
			debug: true,
			fieldmappings: ordereddict.NewDict().
				Set("uri", "x=>x.uri").
				Set("ip_address1", "x=>x.ip_address1").
				Set("ip_address2", "x=>x.ip_address2").
				Set("fieldname", "x=>x.fieldname").
				Set("fieldname_int", "x=>x.fieldname_int").
				Set("CommandLineWide", "x=>x.CommandLineWide").
				Set("CommandLine", "x=>x.CommandLine"),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("uri", "https://1.2.3.4/ecp/default.aspx?__VIEWSTATEGENERATOR=1&__VIEWSTATE=2").
					Set("ip_address1", "192.1.10.1").
					Set("ip_address2", "192.168.0.2").
					Set("fieldname", "needle is a needle").
					Set("fieldname_int", 15).
					// This is a horizontal bar ―
					Set("CommandLine", "ping /f ―g —h ").
					Set("CommandLineWide", base64.StdEncoding.EncodeToString([]byte("p\x00i\x00n\x00g\x00 \x00"))),
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
		{
			description: "One of Condition",
			rule: `
title: One Of
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
  condition: 1 of selection*
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo"),
			debug: true,
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Match", "Should match selection1 and selection2 contains single element").
					Set("Decoded", "jejfjefhellorfriufirtestkdkdg").
					Set("Foo", base64.StdEncoding.EncodeToString([]byte("jejfjefhellorfriufirtestkdkdg"))),
			},
		},
		{
			description: "Test VQL Events",
			rule: `
title: VQL Events
logsource:
  product: windows
  service: application

detection:
  selection1:
     Foo: 1
  selection2:
     Bar|contains: B

  condition: selection1 and selection2

vql: x=>dict(Foo=1, Bar="Baz")
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo").
				Set("Bar", "x=>x.Bar"),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict(),
			},
		},
		{
			description: "Test Conditions",
			rule: `
title: VQL Events
logsource:
  product: windows
  service: application

detection:
  process_creation:
     Proc: 1
  selection_1_1:
     Foo: 1
  selection_1_2:
     Bar|contains: B

  condition: "process_creation and (all of selection_1_* or all of selection_1_*)"
`,
			fieldmappings: ordereddict.NewDict().
				Set("Foo", "x=>x.Foo").
				Set("Bar", "x=>x.Bar").
				Set("Proc", "x=>x.Proc"),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Foo", 1).
					Set("Bar", "Baz").
					Set("Proc", 1),
			},
		},
		{
			description: "Automatic Field Mappings",
			rule: `
title: Automatic Field Mappings
logsource:
  product: windows
  service: application

detection:
   automaticField:
      Foo.Bar.Baz|contains: Hello

   condition: automaticField
`,
			fieldmappings: ordereddict.NewDict(),
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Foo", ordereddict.NewDict().
						Set("Bar", ordereddict.NewDict().
							Set("Baz", "Hello world"))),
			},
		},
	}
)

type SigmaTestSuite struct {
	suite.Suite
}

func (self *SigmaTestSuite) TestSigmaModifiers() {
	result := ordereddict.NewDict()

	ctx := context.Background()
	scope := vql_subsystem.MakeScope().
		AppendVars(ordereddict.NewDict().Set("ScopeVar", "I'm a scope var:"))

	defer scope.Close()

	plugin := SigmaPlugin{}

	for _, test_case := range sigmaTestCases {
		if false && test_case.description != "Automatic Field Mappings" {
			continue
		}

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
			// Ensure the plugin reports the rule that matched and the
			// match object
			_, pres := scope.Associative(row, "_Rule")
			assert.True(self.T(), pres)

			_, pres = scope.Associative(row, "_Match")
			assert.True(self.T(), pres)

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

var (
	loginEvents = []*ordereddict.Dict{
		ordereddict.NewDict().
			Set("Timestamp", "2024-10-10T12:22:00+10").
			Set("EventID", 4625).
			Set("TargetDomainName", "Domain").
			Set("TargetUserName", "A"),
		ordereddict.NewDict().
			Set("Timestamp", "2024-10-10T12:23:00+10").
			Set("EventID", 4625).
			Set("TargetDomainName", "Domain").
			Set("TargetUserName", "B"),
		ordereddict.NewDict().
			Set("Timestamp", "2024-10-10T12:24:00+10").
			Set("EventID", 4625).
			Set("TargetDomainName", "Domain").
			Set("TargetUserName", "A"),
	}

	base_rule_Failed_logon = `
title: Failed logon
name: failed_logon
description: Detect when logon is failed
logsource:
   product: windows
   service: security
detection:
   selection:
     EventID: 4625
   condition: selection
---
`

	loginEvents_field_mappings = ordereddict.NewDict().
		// Default time attribute should be generated by the log source
		Set("Timestamp", "x=>x.Timestamp").
		Set("EventID", "x=>x.EventID").
		Set("SubjectUserName", "x=>x.SubjectUserName").
		Set("TargetDomainName", "x=>x.TargetDomainName").
		Set("TargetUserName", "x=>x.TargetUserName")

	high_priv_enum = `
title: High-privilege group enumeration
name: privileged_group_enumeration
status: stable
logsource:
  product: windows
  service: security
detection:
  selection:
    EventID: 4799
    TargetUserName:
      - Administrators
      - Remote Desktop Users
      - Remote Management Users
      - Distributed COM Users
  condition: selection
level: informational
falsepositives:
  - Administrative activity
  - Directory assessment tools
---
`

	sigmaCorrelationTestCases = []testCase{
		{
			description: "Correlation Test Too few hits",
			rule: base_rule_Failed_logon + `
title: Multiple failed logons for a single user (possible brute force attack)
correlation:
    type: event_count
    rules:
        - failed_logon # Referenced here
    group-by:
        - TargetUserName
        - TargetDomainName
    timespan: 5m
    condition:
        gte: 3
`,
			fieldmappings: loginEvents_field_mappings,

			// Send 2 login events for user A within 5 minutes
			rows: loginEvents,
		}, {
			description: "Correlation Test Right number of hits",
			rule: base_rule_Failed_logon + `
title: Multiple failed logons for a single user (possible brute force attack)
correlation:
    type: event_count
    rules:
        - failed_logon # Referenced here
    group-by:
        - TargetUserName
        - TargetDomainName
    timespan: 5m
    condition:
        gte: 3

# Make sure the details and enrichment comes from the correlation rule.
details: Detected Multiple Failed Logins
enrichment: x=>x._Correlations
`,
			fieldmappings: loginEvents_field_mappings,

			// Send 3 login events for user A within 5 minutes
			rows: append(loginEvents, ordereddict.NewDict().
				Set("Timestamp", "2024-10-10T12:25:00+10").
				Set("EventID", 4625).
				Set("TargetDomainName", "Domain").
				Set("TargetUserName", "A")),
		}, {

			// Example taken from
			// https://sigmahq.io/docs/meta/correlations.html#value-count
			description: "Correlation Test VALUE_COUNT",
			rule: high_priv_enum + `
title: Enumeration of multiple high-privilege groups by tools like BloodHound
status: stable
correlation:
  type: value_count
  rules:
    - privileged_group_enumeration
  group-by:
    - SubjectUserName
  timespan: 15m
  condition:
    gte: 4
    field: TargetUserName
level: high
falsepositives:
  - Administrative activity
  - Directory assessment tools
`,
			fieldmappings: loginEvents_field_mappings,
			rows: []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:22:00+10").
					Set("EventID", 4799).
					Set("SubjectUserName", "admin").
					Set("TargetUserName", "Administrators"),
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:23:00+10").
					Set("EventID", 4799).
					Set("SubjectUserName", "admin").
					Set("TargetUserName", "Remote Desktop Users"),
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:24:00+10").
					Set("EventID", 4799).
					Set("SubjectUserName", "admin").
					Set("TargetUserName", "Remote Management Users"),
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:25:00+10").
					Set("EventID", 4799).
					Set("SubjectUserName", "admin").
					Set("TargetUserName", "Distributed COM Users"),
			},
		}, {

			// Example taken from
			// https://sigmahq.io/docs/meta/correlations.html#temporal
			description: "Correlation Test TEMPORAL",
			rule: `
title: CVE-2023-22518 Exploitation Attempt - Suspicious Confluence Child Process (Windows)
id: 1ddaa9a4-eb0b-4398-a9fe-7b018f9e23db
logsource:
  product: windows # Just for testing
  service: security
detection:
    selection_parent:
        ParentImage|endswith:
            - '\tomcat8.exe'
            - '\tomcat9.exe'
            - '\tomcat10.exe'
        ParentCommandLine|contains: 'confluence'
    selection_child:
        # Note: Only children associated with known campaigns
        - Image|endswith:
              - '\cmd.exe'
              - '\powershell.exe'
        - OriginalFileName:
              - 'Cmd.Exe'
              - 'PowerShell.EXE'
    condition: all of selection_*
---
title: CVE-2023-22518 Exploitation Attempt - Vulnerable Endpoint Connection (Webserver)
id: a902d249-9b9c-4dc4-8fd0-fbe528ef965c
logsource:
  product: windows # Just for testing
  service: security
detection:
    selection_method:
        cs-method: 'POST'
    selection_uris:
        cs-uri-query|contains:
          # Exploitable endpoints
            - '/json/setup-restore-local.action'
            - '/json/setup-restore-progress.action'
            - '/json/setup-restore.action'
            - '/server-info.action'
            - '/setup/setupadministrator.action'
    selection_status:
        # Response code may be indicative of exploitation success, but is not always the case
        sc-status:
            - 200
            - 302
            - 405
    condition: all of selection_*
---
title: CVE-2023-22518 Exploit Chain
correlation:
  type: temporal
  rules:
    - a902d249-9b9c-4dc4-8fd0-fbe528ef965c
    - 1ddaa9a4-eb0b-4398-a9fe-7b018f9e23db
  timespan: 10m
level: high
`,
			fieldmappings: ordereddict.NewDict().
				// Default time attribute should be generated by the log source
				Set("Timestamp", "x=>x.Timestamp").
				Set("ParentImage", "x=>x.ParentImage").
				Set("ParentCommandLine", "x=>x.ParentCommandLine").
				Set("Image", "x=>x.Image").
				Set("OriginalFileName", "x=>x.OriginalFileName").
				Set("cs-method", "x=>x.`cs-method`").
				Set("cs-uri-query", "x=>x.`cs-uri-query`").
				Set("sc-status", "x=>x.`sc-status`"),
			rows: []*ordereddict.Dict{
				// Should trigger 1ddaa9a4-eb0b-4398-a9fe-7b018f9e23db
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:22:00+10").
					Set("ParentImage", "C:\\Windows\\tomcat9.exe").
					Set("ParentCommandLine", "confluence").
					Set("Image", "C:\\Windows\\cmd.exe").
					Set("OriginalFileName", "Cmd.Exe"),

				// Should trigger a902d249-9b9c-4dc4-8fd0-fbe528ef965c
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:23:00+10").
					Set("cs-method", "POST").
					Set("cs-uri-query", "/app//json/setup-restore-local.action").
					Set("sc-status", 200),
			},
		}, {
			description: "Correlation Test TEMPORAL Partial match should not fire",
			rule:        simpleTemporalCorrelationRule,
			fieldmappings: ordereddict.NewDict().
				Set("ParentImage", "x=>x.ParentImage").
				Set("cs-method", "x=>x.`cs-method`"),
			rows: []*ordereddict.Dict{
				// Should trigger r1
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:22:00+10").
					Set("ParentImage", "C:\\Windows\\tomcat8.exe"),

				// Should trigger r1
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:23:00+10").
					Set("ParentImage", "C:\\Windows\\tomcat8.exe"),

				// Should trigger r1
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:24:00+10").
					Set("ParentImage", "C:\\Windows\\tomcat8.exe"),
			},
		},
		{
			description: "Correlation Test TEMPORAL Multiple match",
			rule:        simpleTemporalCorrelationRule,
			// The _Correlations result should show all 3 r1 matches
			// and one r2 match
			fieldmappings: ordereddict.NewDict().
				Set("ParentImage", "x=>x.ParentImage").
				Set("cs-method", "x=>x.`cs-method`"),
			rows: []*ordereddict.Dict{
				// Should trigger r1
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:22:00+10").
					Set("ParentImage", "C:\\Windows\\tomcat8.exe"),

				// Should trigger r1
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:23:00+10").
					Set("ParentImage", "C:\\Windows\\tomcat8.exe"),

				// Should trigger r1
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:24:00+10").
					Set("ParentImage", "C:\\Windows\\tomcat8.exe"),

				// Should trigger r2
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:25:00+10").
					Set("cs-method", "POST"),
			},
		}, {
			description: "Correlation Test TEMPORAL Expired match should not fire",
			rule:        simpleTemporalCorrelationRule,
			fieldmappings: ordereddict.NewDict().
				Set("ParentImage", "x=>x.ParentImage").
				Set("cs-method", "x=>x.`cs-method`"),
			rows: []*ordereddict.Dict{
				// Should trigger r1
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:22:00+10").
					Set("ParentImage", "C:\\Windows\\tomcat8.exe"),

				// Should trigger r2 but more than 10 min later
				ordereddict.NewDict().
					Set("Timestamp", "2024-10-10T12:35:00+10").
					Set("cs-method", "POST"),
			},
		},
	}
)

func (self *SigmaTestSuite) TestSigmaCorrelations() {
	result := ordereddict.NewDict()

	ctx := context.Background()
	scope := vql_subsystem.MakeScope().
		AppendVars(ordereddict.NewDict().Set("ScopeVar", "I'm a scope var:"))

	scope.SetLogger(log.New(os.Stdout, "", 0))

	defer scope.Close()

	plugin := SigmaPlugin{}

	for idx, test_case := range sigmaCorrelationTestCases {
		fmt.Printf("Running case: %v: %v\n", idx, test_case.description)

		if false && idx != 2 {
			continue
		}

		log_collector := &bytes.Buffer{}
		scope.SetLogger(log.New(log_collector, "", 0))

		rows := []types.Row{}
		args := ordereddict.NewDict().
			Set("rules", strings.Split(test_case.rule, "---")).
			Set("log_sources", &LogSourceProvider{
				queries: map[string]types.StoredQuery{
					"*/windows/security": &MockQuery{
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

	goldie.Assert(self.T(), "TestSigmaCorrelation",
		json.MustMarshalIndent(result))
}

func TestSigmaPlugin(t *testing.T) {
	suite.Run(t, &SigmaTestSuite{})
}
