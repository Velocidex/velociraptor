package parsers

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var grokTestCases = []struct {
	name, pattern, data string
}{
	{
		name:    "Multiple Keywords",
		pattern: `(?:%{SYSLOGTIMESTAMP:timestamp}|%{TIMESTAMP_ISO8601:timestamp}) +%{GREEDYDATA:message}`,
		data:    `Mar 27 10:49:14 Ubuntu-22 gdm-autologin]: gkr-pam: no password is available for user`,
	},
}

func TestGrokParser(t *testing.T) {
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()
	result := ordereddict.NewDict()

	for _, testcase := range grokTestCases {
		test_result := GrokParseFunction{}.Call(ctx, scope,
			ordereddict.NewDict().
				Set("grok", testcase.pattern).
				Set("data", testcase.data).
				Set("all_captures", false))
		result.Set(testcase.name, test_result)
	}

	goldie.Assert(t, "TestGrokParser", json.MustMarshalIndent(result))
}
