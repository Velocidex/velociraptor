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
package glob

import (
	"context"
	"errors"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"www.velocidex.com/golang/velociraptor/json"

	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/vfilter"
)

type pathComponentsTestFixtureType struct {
	pattern    string
	components []_PathFilterer
}

var pathComponentsTestFixture = []pathComponentsTestFixtureType{
	{"foo", []_PathFilterer{
		_LiteralComponent{"foo"},
	}},
	{"foo**5", []_PathFilterer{
		_RecursiveComponent{`foo.*\z(?ms)`, 5},
	}},
	{"*.exe", []_PathFilterer{
		&_RegexComponent{regexp: `.*\.exe\z(?ms)`},
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
		if components, err := convert_glob_into_path_components(
			fixture.pattern, MockFileSystemAccessor{}.PathSplit); err == nil {
			if reflect.DeepEqual(fixture.components, components) {
				continue
			}
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
		translated := fnmatch_translate(fixture.pattern)
		if translated != fixture.expected {
			t.Fatalf("Failed to parse %q: %q", translated,
				fixture.expected)
		}
	}
}

type MockFileSystemAccessor []string

func (self MockFileSystemAccessor) New(scope vfilter.Scope) FileSystemAccessor {
	return self
}

func (self MockFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self MockFileSystemAccessor) ReadDir(filepath string) ([]FileInfo, error) {
	seen := []string{}
	_, subpath, _ := self.GetRoot(filepath)
	if !strings.HasSuffix(subpath, "/") {
		subpath = subpath + "/"
	}

	for _, mock_path := range self {
		if strings.HasPrefix(mock_path, subpath) {
			suffix := mock_path[len(subpath):]
			mock_path_components := strings.Split(suffix, "/")
			if !utils.InString(seen, mock_path_components[0]) {
				seen = append(seen, mock_path_components[0])
			}
		}
	}

	var result []FileInfo
	for _, k := range seen {
		result = append(result, vtesting.MockFileInfo{
			Name_:     k,
			FullPath_: path.Join(subpath, k),
		})
	}
	return result, nil
}

func (self MockFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	return nil, errors.New("Not implemented")
}

func (self MockFileSystemAccessor) PathSplit(path string) []string {
	re := regexp.MustCompile("/")
	return re.Split(path, -1)
}

func (self MockFileSystemAccessor) PathJoin(x, y string) string {
	return path.Join(x, y)
}

func (self MockFileSystemAccessor) GetRoot(filepath string) (string, string, error) {
	return "/", path.Clean(filepath), nil
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
}

var fs_accessor = MockFileSystemAccessor{
	"/bin/bash",
	"/bin/dash",
	"/bin/rm",
	"/usr/bin/diff",
	"/usr/sbin/X",
	"/usr/bin/X11/diff",
	"/usr/bin/X11/X11/diff",
	"/usr/bin/X11/X11/X11/diff",
	"/tmp/1",
	"/tmp/1/1.txt",
	"/tmp/1/5",
	"/tmp/1/4",
	"/tmp/1/3",
	"/tmp/1/2",
	"/tmp/1/2/23",
	"/tmp/1/2/21",
	"/tmp/1/2/21/1.txt",
	"/tmp/1/2/21/213",
	"/tmp/1/2/21/212",
	"/tmp/1/2/21/212/1.txt",
	"/tmp/1/2/21/211",
	"/tmp/1/2/20",
}

func TestGlobWithContext(t *testing.T) {
	ctx := context.Background()
	result := make(map[string]interface{})
	for idx, fixture := range _GlobFixture {
		var returned []string

		globber := NewGlobber()
		for _, pattern := range fixture.patterns {
			err := globber.Add(pattern, MockFileSystemAccessor{}.PathSplit)
			if err != nil {
				t.Fatalf("Failed %v", err)
			}
		}

		output_chan := globber.ExpandWithContext(
			ctx, config.GetDefaultConfig(),
			"/", fs_accessor)
		for row := range output_chan {
			returned = append(returned, row.FullPath())
		}

		result[fmt.Sprintf("%03d %s", idx, fixture.name)] = returned
	}

	result_json, _ := json.MarshalIndent(result)
	goldie.Assert(t, "TestGlobWithContext", result_json)
}

func TestBraceExpansion(t *testing.T) {
	var result []string
	globber := NewGlobber()
	globber._brace_expansion("{/bin/{a,b},/usr/bin/{c,d}}/*.exe", &result)
	sort.Strings(result)

	expected := []string{
		"/bin/a/*.exe",
		"/bin/b/*.exe",
		"/usr/bin/c/*.exe",
		"/usr/bin/d/*.exe",
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Failed %v: %v", result, expected)
	}
}
