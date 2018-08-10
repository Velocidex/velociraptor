package glob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sebdah/goldie"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
	"www.velocidex.com/golang/velociraptor/utils"
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
		_RegexComponent{`.*\.exe\z(?ms)`},
	}},
	{"/bin/ls", []_PathFilterer{
		_LiteralComponent{"bin"},
		_LiteralComponent{"ls"},
	}},
	{"/bin/**/ls", []_PathFilterer{
		_LiteralComponent{"bin"},
		_RecursiveComponent{path: `.*\z(?ms)`, depth: 3},
		_LiteralComponent{"ls"},
	}},
}

func TestConvertToPathComponent(t *testing.T) {
	for _, fixture := range pathComponentsTestFixture {
		if components, err := convert_glob_into_path_components(
			fixture.pattern, "/"); err == nil {
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

type MockFileInfo struct {
	name      string
	full_path string
}

func (self MockFileInfo) Name() string       { return self.name }
func (self MockFileInfo) Size() int64        { return 0 }
func (self MockFileInfo) Mode() os.FileMode  { return os.ModePerm }
func (self MockFileInfo) ModTime() time.Time { return time.Time{} }
func (self MockFileInfo) IsDir() bool        { return true }
func (self MockFileInfo) Sys() interface{}   { return nil }
func (self MockFileInfo) FullPath() string   { return self.full_path }
func (self MockFileInfo) Mtime() TimeVal     { return TimeVal{} }
func (self MockFileInfo) Atime() TimeVal     { return TimeVal{} }
func (self MockFileInfo) Ctime() TimeVal     { return TimeVal{} }

type MockFileSystemAccessor []string

func (self MockFileSystemAccessor) PathSep() string { return "/" }

func (self MockFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	seen := []string{}
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	for _, mock_path := range self {
		if strings.HasPrefix(mock_path, path) {
			suffix := mock_path[len(path):]
			mock_path_components := strings.Split(suffix, "/")
			if !utils.InString(&seen, mock_path_components[0]) {
				seen = append(seen, mock_path_components[0])
			}
		}
	}

	var result []FileInfo
	for _, k := range seen {
		result = append(result, MockFileInfo{k, filepath.Join(path, k)})
	}
	return result, nil
}

func (self MockFileSystemAccessor) Open(path string) (io.Reader, error) {
	return nil, errors.New("Not implemented")
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
	{"Depth of 3", []string{"/usr/**/diff"}},
	{"Depth of 4", []string{"/usr/**4/diff"}},
	{"Breadth first traversal", []string{"/tmp/1/*", "/tmp/1/*/*"}},
	{"Breadth first traversal", []string{"/tmp/1/**5"}},
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

		globber := &Globber{}
		for _, pattern := range fixture.patterns {
			err := globber.Add(pattern, "/")
			if err != nil {
				t.Fatalf("Failed %v", err)
			}
		}

		output_chan := globber.ExpandWithContext(ctx, "/", fs_accessor)
		for row := range output_chan {
			returned = append(returned, row.FullPath())
		}

		result[fmt.Sprintf("%03d %s", idx, fixture.name)] = returned
	}

	result_json, _ := json.MarshalIndent(result, "", " ")
	goldie.Assert(t, "TestGlobWithContext", result_json)
}

func TestBraceExpansion(t *testing.T) {
	var result []string
	globber := make(Globber)
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
