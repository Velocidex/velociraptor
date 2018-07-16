package flows

import (
	"encoding/json"
	"github.com/golang/protobuf/jsonpb"
	"github.com/sebdah/goldie"
	"testing"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

var file_finderTests = []struct {
	name string
	args string
}{
	{"Empty request", "{}"},
	{"Just glob", `{"paths": ["/bin/*"]}`},
	{"Mod time", `{"paths": ["/bin/*"],
		"conditions": [{"modification_time": {"min_last_modified_time": 5,
                    "max_last_modified_time": 10}}]}`},

	{"Mod time just min", `{"paths": ["/bin/*"],
		"conditions": [{"modification_time": {"min_last_modified_time": 5}}]}`},

	{"Mod time Error", `{"paths": ["/bin/*"],
		"conditions": [{"modification_time": {"min_last_modified_time": 15,
                    "max_last_modified_time": 10}}]}`},

	{"Access time", `{"paths": ["/bin/*"],
		"conditions": [{"access_time": {"min_last_access_time": 5,
                    "max_last_access_time": 10}}]}`},

	{"Inode time", `{"paths": ["/bin/*"],
		"conditions": [{"inode_change_time": {"min_last_inode_change_time": 5,
                    "max_last_inode_change_time": 10}}]}`},

	{"Size", `{"paths": ["/bin/*"],
		"conditions": [{"size": {"min_file_size": 500,
                    "max_file_size": 1000}}]}`},

	{"Grep", `{"paths": ["/bin/*"],
		"conditions": [{"contents_literal_match": {"literal": "aGVsbG8="}}]}`},

	{"Grep and simple", `{"paths": ["/bin/*"],
		"conditions": [{"contents_literal_match": {"literal": "aGVsbG8="}},
                               {"modification_time": {"min_last_modified_time": 15}}]}`},

	{"Upload simple", `{"paths": ["/bin/*"],
		"conditions": [{"modification_time": {"min_last_modified_time": 15}}],
                "action": {"download": {"max_size": 500}}}`},
}

type fixtureResult struct {
	Name          string
	Error         string
	Request       *flows_proto.FileFinderArgs
	CollectorArgs *actions_proto.VQLCollectorArgs
}

func TestCompilerFileFinderArgs(t *testing.T) {
	result := []*fixtureResult{}
	for _, test_case := range file_finderTests {
		file_finder_args := &flows_proto.FileFinderArgs{}
		err := jsonpb.UnmarshalString(test_case.args, file_finder_args)
		if err != nil {
			t.Fatalf(err.Error())
		}
		builder := file_finder_builder{args: file_finder_args}
		collector_args, err := builder.Build()
		test_result := &fixtureResult{
			Name:          test_case.name,
			Request:       file_finder_args,
			CollectorArgs: collector_args,
		}
		if err != nil {
			test_result.Error = err.Error()
		}
		result = append(result, test_result)
	}

	s, err := json.MarshalIndent(result, "", " ")
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}

	goldie.Assert(t, "compileFileFinderArgs", s)
}
