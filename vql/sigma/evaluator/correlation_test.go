package evaluator

import (
	"testing"

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
