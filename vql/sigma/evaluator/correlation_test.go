package evaluator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func TestGetCmp(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	test_cases := []struct {
		description string
		condition   map[string]interface{}
		// Expected result for count = 0, 1, 2, 3, 4, 5
		expected []bool
	}{
		{"No condition", nil,
			[]bool{true, true, true, true, true, true}},
		{"gt", map[string]interface{}{"gt": 3},
			[]bool{false, false, false, false, true, true}},
		{"gte", map[string]interface{}{"gte": 3},
			[]bool{false, false, false, true, true, true}},
		{"lt", map[string]interface{}{"lt": 3},
			[]bool{true, true, true, false, false, false}},
		{"lte", map[string]interface{}{"lte": 3},
			[]bool{true, true, true, true, false, false}},
		{"eq", map[string]interface{}{"eq": 3},
			[]bool{false, false, false, true, false, false}},
		{"neq", map[string]interface{}{"neq": 3},
			[]bool{true, true, true, false, true, true}},
		{"Range gte lte", map[string]interface{}{"gte": 2, "lte": 3},
			[]bool{false, false, true, true, false, false}},
		{"Range gt lt", map[string]interface{}{"gt": 1, "lt": 4},
			[]bool{false, false, true, true, false, false}},
		{"value_count field is ignored",
			map[string]interface{}{"field": "TargetUserName", "gt": 2},
			[]bool{false, false, false, true, true, true}},
	}

	for _, test_case := range test_cases {
		cmp, err := getCmp(scope, test_case.condition)
		assert.NoError(t, err, test_case.description)

		for count, expected := range test_case.expected {
			assert.Equal(t, expected, cmp(count),
				"%v: count %v", test_case.description, count)
		}
	}
}

func TestGetCmpUnknownOperator(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	for _, condition := range []map[string]interface{}{
		{"gtt": 3},
		{"gte": 3, "foo": 1},
	} {
		_, err := getCmp(scope, condition)
		assert.ErrorContains(t, err, "unsupported correlation condition")
	}
}

func TestParseTimespan(t *testing.T) {
	test_cases := []struct {
		timespan string
		expected time.Duration
	}{
		{"", 5 * time.Minute},
		{"30s", 30 * time.Second},
		{"10m", 10 * time.Minute},
		{"2h", 2 * time.Hour},
		{"1d", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
	}

	for _, test_case := range test_cases {
		timespan, err := parseTimespan(test_case.timespan)
		assert.NoError(t, err, test_case.timespan)
		assert.Equal(t, test_case.expected, timespan, test_case.timespan)
	}

	for _, timespan := range []string{"d", "-1d", "1.5d", "1x", "foo"} {
		_, err := parseTimespan(timespan)
		assert.Error(t, err, timespan)
	}
}
