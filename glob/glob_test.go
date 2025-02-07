/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
package glob

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/config"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type pathComponentsTestFixtureType struct {
	pattern    string
	components []_PathFilterer
}

var pathComponentsTestFixture = []pathComponentsTestFixtureType{
	{"foo", []_PathFilterer{
		_LiteralComponent{"foo"},
	}},
	// A ** has to start at the begining of the component, otherwise
	// it is not considered a recursive component and just interpreted
	// as a normal wild card.
	{"foo**", []_PathFilterer{
		NewRegexComponent(`foo.*.*\z(?ms)`),
	}},
	{"**5", []_PathFilterer{
		_RecursiveComponent{`.*\z(?ms)`, 5},
	}},
	{"*.exe", []_PathFilterer{
		NewRegexComponent(`.*\.exe\z(?ms)`),
	}},
	{"/bin/ls", []_PathFilterer{
		_LiteralComponent{"bin"},
		_LiteralComponent{"ls"},
	}},
	{"/bin/**/ls", []_PathFilterer{
		_LiteralComponent{"bin"},
		_RecursiveComponent{path: `.*\z(?ms)`, depth: 30},
		_LiteralComponent{"ls"},
	}},
}

func TestConvertToPathComponent(t *testing.T) {
	for _, fixture := range pathComponentsTestFixture {
		components, err := convert_glob_into_path_components(
			accessors.MustNewLinuxOSPath(fixture.pattern))
		if err == nil {
			if reflect.DeepEqual(fixture.components, components) {
				continue
			}
			utils.DlvBreak()
			t.Fatalf("Unexpected %v: %v",
				fixture.components, components)
		}
		t.Fatalf("Failed to parse %v", fixture.pattern)
	}
}

type fnmatchTranslateType struct {
	pattern  string
	expected string
}

var fnmatchTranslateTypeFixture = []fnmatchTranslateType{
	{"foo", "foo\\z(?ms)"},
	{"[^a[]foo", "[\\^a[]foo\\z(?ms)"},
	{"*.txt", ".*\\.txt\\z(?ms)"},
	{"foo?bar", "foo.bar\\z(?ms)"},
}

func TestFnMatchTranslate(t *testing.T) {
	for _, fixture := range fnmatchTranslateTypeFixture {
		translated := FNmatchTranslate(fixture.pattern)
		if translated != fixture.expected {
			t.Fatalf("Failed to parse %q: %q", translated,
				fixture.expected)
		}
	}
}

var _GlobFixture = []struct {
	name     string
	patterns []string
}{
	{"Case insensitive", []string{"/bin/Bash"}},
	{"Character class", []string{"/bin/[a-b]ash"}},
	{"Inverted range", []string{"/bin/[!a-b]ash"}},
	{"Brace expansion.", []string{"/bin/{b,d}ash"}},
	{"Depth of 2", []string{"/usr/**2/diff"}},
	{"Depth of 30", []string{"/usr/**/diff"}},
	{"Depth of 4", []string{"/usr/**4/diff"}},
	{"Breadth first traversal", []string{"/tmp/1/*", "/tmp/1/*/*"}},
	{"Breadth first traversal", []string{"/tmp/1/**5"}},
	{"Recursive matches zero or more", []string{"/usr/bin/X11/**/diff"}},
	{"Recursive matches none at end", []string{"/bin/bash/**"}},
	{"Match masked by two matches", []string{"/usr/bin", "/usr/*/diff"}},
	{"Multiple globs matching same file", []string{"/bin/bash", "/bin/ba*"}},

	// One valid glob and one invalid glob - we should just ignore the invalid glob.
	{"Invalid globs", []string{"/bin/bash", "/bin/\xa0*"}},
}

func GetMockFileSystemAccessor() accessors.FileSystemAccessor {
	root_path := accessors.MustNewLinuxOSPath("")
	result := accessors.NewVirtualFilesystemAccessor(root_path)
	for _, path := range []string{
		"/bin/bash",
		"/bin/dash",
		"/bin/rm",
		"/usr/bin/diff",
		"/usr/sbin/X",
		"/usr/bin/X11/diff",
		"/usr/bin/X11/X11/diff",
		"/usr/bin/X11/X11/X11/diff",
		"/tmp/1/",
		"/tmp/1/1.txt",
		"/tmp/1/5/",
		"/tmp/1/4/",
		"/tmp/1/3/",
		"/tmp/1/2/",
		"/tmp/1/2/23/",
		"/tmp/1/2/21/",
		"/tmp/1/2/21/1.txt",
		"/tmp/1/2/21/213/",
		"/tmp/1/2/21/212/",
		"/tmp/1/2/21/212/1.txt",
		"/tmp/1/2/21/211/",
		"/tmp/1/2/20/",
	} {
		result.SetVirtualFileInfo(&accessors.VirtualFileInfo{
			Path:    accessors.MustNewLinuxOSPath(path),
			RawData: []byte("A"),
			IsDir_:  strings.HasSuffix(path, "/"),
		})
	}

	return result
}

func TestGlobWithContext(t *testing.T) {
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()

	fs_accessor := GetMockFileSystemAccessor()

	result := ordereddict.NewDict()
	for idx, fixture := range _GlobFixture {
		var returned []string

		globber := NewGlobber()
		defer globber.Close()

		patterns := ExpandBraces(fixture.patterns)

		for _, pattern := range patterns {
			err := globber.Add(accessors.MustNewLinuxOSPath(pattern))
			if err != nil {
				fmt.Printf("While adding %v: %v\n", pattern, err)
			}
		}

		output_chan := globber.ExpandWithContext(
			ctx, scope, config.GetDefaultConfig(),
			accessors.MustNewLinuxOSPath("/"), // root
			fs_accessor)
		for row := range output_chan {
			hit := row.(*GlobHit)
			globs := hit.Globs()
			sort.Strings(globs)
			returned = append(returned,
				fmt.Sprintf("%v Data: %v\n", hit.OSPath(), globs))
		}

		result.Set(fmt.Sprintf("%03d %s %s", idx, fixture.name,
			strings.Join(fixture.patterns, " , ")), returned)
	}

	result_json, _ := json.MarshalIndent(result)
	goldie.Assert(t, "TestGlobWithContext", result_json)
}

func TestBraceExpansion(t *testing.T) {
	result := ExpandBraces([]string{"/{bin/ls*,usr*/top}"})
	expected := []string{
		"/bin/ls*",
		"/usr*/top",
	}

	assert.Equal(t, 2, len(result))
	for idx, e := range result {
		assert.Equal(t, e, expected[idx])
	}
}

func NewRegexComponent(re string) *_RegexComponent {
	res := &_RegexComponent{regexp: re}
	res.compiled = regexp.MustCompile("^(?msi)" + res.regexp)
	return res
}
