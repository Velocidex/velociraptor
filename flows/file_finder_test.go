// +build deprecated

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package flows

import (
	"encoding/json"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	"github.com/sebdah/goldie"
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
