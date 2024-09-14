package uploads

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var (
	testRanges = []*Range{
		{Offset: 10, Length: 10},
		{Offset: 50, Length: 10},
		{Offset: 80, Length: 10},
	}
)

func TestGetNextRange(t *testing.T) {
	fixture := []*ordereddict.Dict{}
	for _, testcase := range []int64{0, 10, 15, 85, 90, 100} {
		current_range, next_range := GetNextRange(testcase, testRanges)
		fixture = append(fixture, ordereddict.NewDict().
			Set("TestCase", testcase).
			Set("current_range", current_range).
			Set("next_range", next_range))
	}

	goldie.Assert(t, "TestGetNextRange", json.MustMarshalIndent(fixture))
}
