package glob

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
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
func (self MockFileInfo) IsDir() bool        { return false }
func (self MockFileInfo) Sys() interface{}   { return nil }
func (self MockFileInfo) FullPath() string   { return self.full_path }

type MockFileSystemAccessor []string

func (self MockFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	seen := make(map[string]bool)
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	for _, mock_path := range self {
		if strings.HasPrefix(mock_path, path) {
			suffix := mock_path[len(path):]
			mock_path_components := strings.Split(suffix, "/")
			seen[mock_path_components[0]] = true
		}
	}

	var result []FileInfo
	for k, _ := range seen {
		result = append(result, MockFileInfo{k, filepath.Join(path, k)})
	}
	return result, nil
}

type _GlobFixtureType struct {
	patterns []string
	expected []string
}

var _GlobFixture = []_GlobFixtureType{
	// Case insensitive matching.
	{[]string{"/bin/Bash"}, []string{"/bin/bash"}},

	// Range matching.
	{[]string{"/bin/[a-b]ash"}, []string{"/bin/bash"}},

	// Inverted range.
	{[]string{"/bin/[!a-b]ash"}, []string{"/bin/dash"}},

	// Brace expansion.
	{[]string{"/bin/{b,d}ash"}, []string{
		"/bin/bash",
		"/bin/dash",
	}},

	// Depth of 2
	{[]string{"/usr/**2/diff"}, []string{
		"/usr/bin/X11/diff",
		"/usr/bin/diff",
	}},

	// Depth of 3 (default)
	{[]string{"/usr/**/diff"}, []string{
		"/usr/bin/X11/X11/diff",
		"/usr/bin/X11/diff",
		"/usr/bin/diff",
	}},

	// Depth of 4 (default)
	{[]string{"/usr/**4/diff"}, []string{
		"/usr/bin/X11/X11/X11/diff",
		"/usr/bin/X11/X11/diff",
		"/usr/bin/X11/diff",
		"/usr/bin/diff",
	}},
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
}

func TestGlob(t *testing.T) {
	for _, fixture := range _GlobFixture {
		tree := NewGlobber()

		for _, pattern := range fixture.patterns {
			err := tree.Add(pattern, "/")
			if err != nil {
				t.Fatalf("Failed %v", err)
			}
		}

		expanded := tree.Expand("/", fs_accessor)
		sort.Strings(expanded)

		if !reflect.DeepEqual(expanded, fixture.expected) {
			t.Fatalf("Failed %v: %v", expanded,
				fixture.expected)
		}
	}
}

func TestGlobWithContext(t *testing.T) {
	ctx := context.Background()
	for _, fixture := range _GlobFixture {
		var returned []string

		globber := make(Globber)
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
		sort.Strings(returned)
		if !reflect.DeepEqual(returned, fixture.expected) {
			t.Fatalf("Failed %v: %v", returned,
				fixture.expected)
		}
	}
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
