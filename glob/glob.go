package glob

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The algorithm in this file is based on the Rekall algorithm here:
// https://github.com/google/rekall/blob/master/rekall-core/rekall/plugins/response/files.py#L255

type FileInfo interface {
	os.FileInfo
	FullPath() string
}

type OSFileInfo struct {
	os.FileInfo
	_full_path string
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path
}

func (self *OSFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Sys      interface{}
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Sys:      self.Sys(),
	})

	return result, err
}

func (u *OSFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

// Interface for accessing the filesystem. Used for dependency
// injection.
type FileSystemAccessor interface {
	ReadDir(path string) ([]FileInfo, error)
}

// Real implementation.
// TODO: Support windows properly.
type OSFileSystemAccessor struct{}

func (self OSFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	files, err := ioutil.ReadDir(path)
	if err == nil {
		var result []FileInfo
		for _, f := range files {
			result = append(result,
				&OSFileInfo{f, filepath.Join(path, f.Name())})
		}
		return result, nil
	}
	return nil, err
}

type _PathFilterer interface {
	Match(f FileInfo) bool
}

// A sentinel is used to determine if we should report this file. At
// each level of walking the Match tree we check if a _Sentinel is
// present at this level. If it is then we need to report this.
type _Sentinel struct{}

func (self _Sentinel) Match(f FileInfo) bool {
	return true
}

var (
	sentinal_filter = _Sentinel{}
	sentinal        = &Globber{sentinal_filter: nil}
)

type _RecursiveComponent struct {
	path  string
	depth int
}

func (self _RecursiveComponent) Match(f FileInfo) bool {
	return false
}

type _RegexComponent struct {
	regexp string
}

func (self _RegexComponent) Match(f FileInfo) bool {
	re := regexp.MustCompile("^(?msi)" + self.regexp)
	return re.MatchString(f.Name())
}

type _LiteralComponent struct {
	path string
}

func (self _LiteralComponent) Match(f FileInfo) bool {
	return strings.EqualFold(self.path, f.Name())
}

// A tree of filters - each filter branches to a subfilter.
type Globber map[_PathFilterer]*Globber

// A factory for a new Globber. To use the globber simply Add()
// any patterns and call Expand() using a suitable FileSystemAccessor.
func NewGlobber() Globber {
	return make(Globber)
}

// Add a new pattern to the filter tree.
func (self *Globber) Add(pattern string, pathsep string) error {
	var brace_expanded []string
	self._brace_expansion(pattern, &brace_expanded)

	for _, expanded := range brace_expanded {
		err := self._add_brace_expanded(expanded, pathsep)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Globber) _add_brace_expanded(pattern string, pathsep string) error {
	// Convert the pattern into path components.
	filter, err := convert_glob_into_path_components(pattern, pathsep)
	if err == nil {
		// Expand path components into alternatives
		return self._expand_path_components(filter)

	} else {
		return err
	}
}

func (self *Globber) _brace_expansion(pattern string, result *[]string) {
	groups := _GROUPING_PATTERN.FindStringSubmatch(pattern)
	if len(groups) > 0 {
		left := groups[1]
		middle := strings.Split(groups[2], ",")
		right := groups[3]

		for _, item := range middle {
			self._brace_expansion(left+item+right, result)
		}
	} else if !utils.InString(result, pattern) {
		*result = append(*result, pattern)
	}
}

// Adds the raw filter into the Globber tree. This is called
// after any expansion.
func (self *Globber) _add_filter(components []_PathFilterer) error {
	var current *Globber = self

	for _, element := range components {
		next, pres := (*current)[element]
		if pres {
			current = next
		} else {
			next := &Globber{}
			(*current)[element] = next
			current = next
		}
	}

	// Add Sentinal to ensure matches are reported here.
	(*current)[sentinal_filter] = nil
	return nil
}

// Expands the component tree by traversing the filesystem. This
// version uses a context to allow cancellation. We write the FileInfo
// into the output channel.
func (self Globber) ExpandWithContext(
	ctx context.Context, path string,
	accessor FileSystemAccessor) <-chan FileInfo {
	output_chan := make(chan FileInfo)

	go func() {
		defer close(output_chan)

		// We want to do a breadth first recursion - not a
		// depth first recursion. This ensures that readers of
		// the results can detect all the hits in a particular
		// directory before processing its children.
		children := make(map[string]*Globber)

		// Walk the filter tree. List the directory and for each file
		// that matches a filter at this level, recurse into the next
		// level.
		files, err := accessor.ReadDir(path)
		if err != nil {
			return
		}

		result := []FileInfo{}

		// For each file that matched, we check which component
		// would match it.
		for _, f := range files {
			for filterer, next := range self {
				if !filterer.Match(f) {
					continue
				}

				if next == nil {
					continue
				}

				_, next_has_sentinal := (*next)[sentinal_filter]
				if next_has_sentinal {
					result = append(result, f)
				}

				// Only recurse into directories.
				if f.IsDir() {
					next_path := filepath.Join(path, f.Name())
					children[next_path] = next
				}
			}
		}

		// Sort the results alphabetically.
		sort.Slice(result, func(i, j int) bool {
			return -1 == strings.Compare(
				result[i].FullPath(),
				result[j].FullPath())
		})
		for _, f := range result {
			output_chan <- f
		}

		for next_path, next := range children {
			child_chan := next.ExpandWithContext(ctx, next_path, accessor)

		search_subdir:
			for {
				select {
				case <-ctx.Done():
					return

				case f, ok := <-child_chan:
					if !ok {
						break search_subdir
					}
					output_chan <- f
				}
			}
		}
	}()

	return output_chan
}

func (self Globber) _expand_path_components(filter []_PathFilterer) error {
	// Create a new filter with simplified elements.
	var new_filter []_PathFilterer
	for idx, item := range filter {
		// Convert a recursive path component into a
		// series of regex components.
		// e.g.  /foo/**3/bar  -> {"foo/*/bar",
		//                         "foo/*/*/bar",
		//                         "foo/*/*/*/bar"}
		if t, pres := item.(_RecursiveComponent); pres {
			left := new_filter
			right := filter[idx+1:]
			var middle []_PathFilterer

			for i := 0; i < t.depth; i++ {
				middle = append(middle, _RegexComponent{".*"})
				new_filter := append(left, middle...)
				new_filter = append(new_filter, right...)

				// Expand each component further.
				err := self._expand_path_components(new_filter)
				if err != nil {
					return err
				}
			}

			return nil
		} else {
			new_filter = append(new_filter, item)
		}
	}

	// If we get here the new_filter should be clean and
	// need no expansions.
	self._add_filter(new_filter)

	return nil
}

var (
	_INTERPOLATED_REGEX = regexp.MustCompile("%%([^%]+?)%%")

	// Support Brace Expansion {a,b}. NOTE: This happens before wild card
	// expansions so you can do /foo/bar/{*.exe,*.dll}
	_GROUPING_PATTERN = regexp.MustCompile("^(.*){([^}]+)}(.*)$")
	_RECURSION_REGEX  = regexp.MustCompile("\\*\\*(\\d*)")

	// A regex indicating if there are shell globs in this path.
	_GLOB_MAGIC_CHECK = regexp.MustCompile("[*?[]")
)

// Converts a glob pattern into a list of pathspec components.
// Wildcards are also converted to regular expressions. The pathspec
// components do not span directories, and are marked as a regex or a
// literal component.
// We also support recursion into directories using the ** notation.  For
// example, /home/**2/foo.txt will find all files named foo.txt recursed 2
// directories deep. If the directory depth is omitted, it defaults to 3.

// Example:
// /home/test**/*exe -> [{path: 'home', type: "LITERAL",
//                       {path: 'test.*\\Z(?ms)', type: "RECURSIVE",
// 			 {path: '.*exe\\Z(?ms)', type="REGEX"}]]
func convert_glob_into_path_components(pattern string, path_sep string) (
	[]_PathFilterer, error) {
	var result []_PathFilterer

	for _, path_component := range strings.Split(pattern, path_sep) {
		if len(path_component) == 0 {
			continue
		}

		// A ** in the path component means recurse into directories that
		// match the pattern.
		if groups := _RECURSION_REGEX.FindStringSubmatch(
			path_component); len(groups) > 0 {
			depth := 3

			// Allow the user to override the recursion depth.
			if len(groups[1]) > 0 {
				var err error
				if depth, err = strconv.Atoi(groups[1]); err != nil {
					return nil, err
				}
			}

			result = append(result, _RecursiveComponent{
				path: fnmatch_translate(strings.Replace(
					path_component, groups[0], "*", 1)),
				depth: depth,
			})

		} else if m := _GLOB_MAGIC_CHECK.FindString(path_component); len(m) > 0 {
			result = append(result, _RegexComponent{
				regexp: fnmatch_translate(path_component),
			})
		} else {
			result = append(result, _LiteralComponent{
				path: path_component,
			})
		}
	}
	return result, nil
}

type unicode []rune

// Copied from Python's fnmatch.translate
/*
   Translate a shell PATTERN to a regular expression.

   There is no way to quote meta-characters.
*/
func fnmatch_translate(pat string) string {
	unicode_pat := unicode(pat)
	n := len(unicode_pat)
	res := unicode("")

	for i := 0; i < n; {
		c := unicode_pat[i]
		i = i + 1
		if c == '*' {
			res = append(res, unicode(".*")...)
		} else if c == '?' {
			res = append(res, unicode(".")...)
		} else if c == '[' {
			j := i
			if j < n && unicode_pat[j] == '!' {
				j = j + 1
			}
			if j < n && unicode_pat[j] == ']' {
				j = j + 1
			}
			for j < n {
				if unicode_pat[j] == ']' {
					break
				}

				j = j + 1
			}
			if j >= n {
				res = append(res, '\\', '[')
			} else {
				stuff := escape_backslash(unicode_pat[i:j])

				i = j + 1
				if stuff[0] == '!' {
					stuff = append(unicode("^"), stuff[1:]...)
				} else if stuff[0] == '^' {
					stuff = append(unicode("\\"), stuff...)
				}

				res = append(res, '[')
				res = append(res, stuff...)
				res = append(res, ']')
			}
		} else {
			res = append(res, escape_rune(c)...)
		}
	}

	res = append(res, unicode("\\z(?ms)")...)
	return string(res)
}

// Same as python's re.escape()
func escape_rune(x rune) unicode {
	var result unicode

	i := int(x)

	if !(int('a') <= i && i <= int('z') ||
		int('A') <= i && i <= int('Z') ||
		int('0') <= i && i <= int('9')) {
		result = append(result, '\\')
	}

	return append(result, x)
}

func escape_backslash(pattern unicode) unicode {
	var result unicode

	for _, x := range pattern {
		if x == '\\' {
			result = append(result, '\\')
		}
		result = append(result, x)
	}

	return result
}
