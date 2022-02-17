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
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The algorithm in this file is based on the Rekall algorithm here:
// https://github.com/google/rekall/blob/master/rekall-core/rekall/plugins/response/files.py#L255
type FileDev interface {
	Dev() uint64
}

type _PathFilterer interface {
	Match(f accessors.FileInfo) bool
}

// A sentinel is used to determine if we should report this file. At
// each level of walking the Match tree we check if a _Sentinel is
// present at this level. If it is then we need to report this.
type _Sentinel struct{}

func (self _Sentinel) Match(f accessors.FileInfo) bool {
	return true
}

func (self _Sentinel) String() string {
	return "Sentinel"
}

var (
	sentinal_filter = _Sentinel{}
)

type _RecursiveComponent struct {
	path  string
	depth int
}

func (self _RecursiveComponent) Match(f accessors.FileInfo) bool {
	return false
}

type _RegexComponent struct {
	regexp   string
	compiled *regexp.Regexp
}

func (self *_RegexComponent) Match(f accessors.FileInfo) bool {
	if self.compiled == nil {
		self.compiled = regexp.MustCompile("^(?msi)" + self.regexp)
	}

	return self.compiled.MatchString(f.Name())
}

func (self _RegexComponent) String() string {
	return "re:" + self.regexp
}

type _LiteralComponent struct {
	path string
}

func (self _LiteralComponent) String() string {
	return self.path
}

func (self _LiteralComponent) Match(f accessors.FileInfo) bool {
	return strings.EqualFold(self.path, f.Name())
}

type GlobOptions struct {
	// Do not follow symbolic links.
	DoNotFollowSymlinks bool

	// Stay on the one filesystem
	OneFilesystem bool

	// Allow the user to control which directory we descend into.
	RecursionCallback func(file_info accessors.FileInfo) bool
}

// A tree of filters - each filter branches to a subfilter.
type Globber struct {
	filters map[_PathFilterer]*Globber
	options GlobOptions
}

func (self *Globber) WithOptions(options GlobOptions) *Globber {
	self.options = options

	return self
}

// A factory for a new Globber. To use the globber simply Add() any
// patterns and call ExpandWithContext() using a suitable
// FileSystemAccessor.
func NewGlobber() *Globber {
	return &Globber{
		filters: make(map[_PathFilterer]*Globber),
	}
}

func (self Globber) DebugString() string {
	return self._DebugString("")
}

func (self Globber) _DebugString(indent string) string {
	re := regexp.MustCompile("^")

	result := []string{}
	for k, v := range self.filters {
		if v == nil {
			continue
		}
		subtree := re.ReplaceAllString(v._DebugString(indent+"  "), indent)
		result = append(result, fmt.Sprintf("%s: %s\n", k, subtree))
	}

	return strings.Join(result, "\n")
}

// Add a new pattern to the filter tree.
func (self *Globber) Add(pattern *accessors.OSPath) error {
	// Convert the pattern into path components.
	filter, err := convert_glob_into_path_components(pattern)
	if err == nil {
		// Expand path components into alternatives
		return self._expand_path_components(filter, 0)

	} else {
		return err
	}
}

// Adds the raw filter into the Globber tree. This is called
// after any expansion.
func (self *Globber) _add_filter(components []_PathFilterer) error {
	var current *Globber = self

	for _, element := range components {
		next, pres := current.filters[element]
		if pres {
			current = next
		} else {
			next := NewGlobber().WithOptions(self.options)
			current.filters[element] = next
			current = next
		}
	}

	// Add Sentinal to ensure matches are reported here.
	current.filters[sentinal_filter] = nil
	return nil
}

func (self *Globber) is_dir_or_link(
	f accessors.FileInfo, accessor accessors.FileSystemAccessor, depth int) bool {
	// Do not follow symlinks to symlinks deeply.
	if depth > 10 {
		return false
	}

	// Allow the callers to control our symlink behavior.
	if self.options.RecursionCallback != nil &&
		!self.options.RecursionCallback(f) {
		return false
	}

	// If it is a link we need to determine if the target is a
	// directory.
	if f.IsLink() {
		if self.options.DoNotFollowSymlinks {
			return false
		}

		target, err := f.GetLink()
		if err == nil {
			target_info, err := accessor.Lstat(target.String())
			if err == nil {
				// Check if the target is on a different filesystem
				// than the current file
				if self.options.OneFilesystem {
					current_dev, ok := DevOf(f)
					if ok {
						target_dev, ok := DevOf(target_info)
						if ok && current_dev != target_dev {
							return false
						}
					}
				}

				return self.is_dir_or_link(target_info, accessor, depth+1)
			}

			// Hmm we failed to lstat the target - assume
			// it is a directory anyway.
			return true
		}
	}

	if f.IsDir() {
		return true
	}

	return false
}

// Expands the component tree by traversing the filesystem. This
// version uses a context to allow cancellation. We write the FileInfo
// into the output channel.
func (self *Globber) ExpandWithContext(
	ctx context.Context,
	config_obj *config_proto.Config,
	root *accessors.OSPath,
	accessor accessors.FileSystemAccessor) <-chan accessors.FileInfo {
	output_chan := make(chan accessors.FileInfo)

	go func() {
		defer close(output_chan)

		// We want to do a breadth first recursion - not a
		// depth first recursion. This ensures that readers of
		// the results can detect all the hits in a particular
		// directory before processing its children.
		children := make(map[string][]*Globber)

		// Walk the filter tree. List the directory and for each file
		// that matches a filter at this level, recurse into the next
		// level.
		files, err := accessor.ReadDirWithOSPath(root)
		if err != nil {
			logging.GetLogger(config_obj, &logging.GenericComponent).
				Debug("Globber.ExpandWithContext: %v while processing %v",
					err, root.String())
			return
		}

		result := []accessors.FileInfo{}

		// For each file that matched, we check which component
		// would match it.
		for _, f := range files {
			for filterer, next := range self.filters {
				if !filterer.Match(f) {
					continue
				}
				if next == nil {
					continue
				}
				_, next_has_sentinal := next.filters[sentinal_filter]
				if next_has_sentinal {
					result = append(result, f)
				}

				// Only recurse into directories.
				if self.is_dir_or_link(f, accessor, 0) {
					name := f.Name()
					item := []*Globber{next}
					prev_item, pres := children[name]
					if pres {
						item = append(prev_item, next)
					}
					children[name] = item
				}
			}
		}

		// Sort the results alphabetically.
		sort.Slice(result, func(i, j int) bool {
			return -1 == strings.Compare(
				result[i].OSPath().Basename(),
				result[j].OSPath().Basename())
		})
		for _, f := range result {
			select {
			case <-ctx.Done():
				return

			case output_chan <- f:
			}
		}

		for name, nexts := range children {
			next_path := root.Append(name)
			for _, next := range nexts {
				// There is no point expanding this
				// node if it is just a sentinal -
				// special case it for efficiency.
				if is_sentinal(next) {
					continue
				}
				for f := range next.ExpandWithContext(
					ctx, config_obj, next_path, accessor) {
					select {
					case <-ctx.Done():
						return

					case output_chan <- f:
					}
				}
			}
		}
	}()

	return output_chan
}

func is_sentinal(globber *Globber) bool {
	if len(globber.filters) != 1 {
		return false
	}

	for k, v := range globber.filters {
		if k == sentinal_filter && v == nil {
			return true
		}
	}

	return false
}

func (self Globber) _expand_path_components(
	filter []_PathFilterer, depth int) error {

	// Create a new filter with simplified elements.
	var new_filter []_PathFilterer
	for idx, item := range filter {
		// Convert a recursive path component into a series of
		// regex components. Note ** also matches zero
		// intermediate paths.
		// e.g.  /foo/**3/bar  -> {"foo/bar",
		//                         "foo/*/bar",
		//                         "foo/*/*/bar",
		//                         "foo/*/*/*/bar"}
		switch t := item.(type) {
		case _RecursiveComponent:
			left := new_filter
			right := filter[idx+1:]
			var middle []_PathFilterer

			for i := 0; i <= t.depth; i++ {
				new_filter := append(left, middle...)
				new_filter = append(new_filter, right...)

				// Only add the zero match if there is
				// anything to our right. This
				// prevents /foo/** matching /foo but
				// allows /foo/**/bar matching
				// /foo/bar
				if (len(middle) == 0 && len(right) > 0) ||
					len(middle) > 0 {
					// Expand each component further.
					err := self._expand_path_components(new_filter, depth+1)
					if err != nil {
						return err
					}
				}
				middle = append(middle, &_RegexComponent{
					regexp: FNmatchTranslate("*"),
				})
			}

			return nil

		default:
			new_filter = append(new_filter, item)
		}
	}

	// If we get here the new_filter should be clean and
	// need no expansions.
	return self._add_filter(new_filter)
}

var (
	// Support Brace Expansion {a,b}. NOTE: This happens before wild card
	// expansions so you can do /foo/bar/{*.exe,*.dll}.

	// Note: Since alternate syntax is similar to Pathspecs (which are
	// plain JSON dicts) we need to tell them apart. Therefore we do
	// not accept an alternate group at the first character. A path
	// separator can always be added to disambiguate. For example:
	// This is ok: /{/bin/ls,/bin/rm}
	// This is not ok: {/bin/ls,/bin/rm}
	_GROUPING_PATTERN = regexp.MustCompile("^(.+)[{]([^{}]+)[}](.*)$")
	_RECURSION_REGEX  = regexp.MustCompile(`\*\*(\d*)`)

	// A regex indicating if there are shell globs in this path.
	_GLOB_MAGIC_CHECK = regexp.MustCompile("[*?[]")
)

// Converts a glob pattern into a list of pathspec components.
// Wildcards are also converted to regular expressions. The pathspec
// components do not span directories, and are marked as a regex or a
// literal component.
// We also support recursion into directories using the ** notation.  For
// example, /home/**2/foo.txt will find all files named foo.txt recursed 2
// directories deep. If the directory depth is omitted, it defaults to 30.

// Example:
// /home/test**/*exe -> [{path: 'home', type: "LITERAL",
//                       {path: 'test.*\\Z(?ms)', type: "RECURSIVE",
// 			 {path: '.*exe\\Z(?ms)', type="REGEX"}]]
func convert_glob_into_path_components(pattern *accessors.OSPath) (
	[]_PathFilterer, error) {
	var result []_PathFilterer

	for _, path_component := range pattern.Components {
		if len(path_component) == 0 {
			continue
		}

		// A ** in the path component means recurse into directories that
		// match the pattern.
		if groups := _RECURSION_REGEX.FindStringSubmatch(
			path_component); len(groups) > 0 {

			// Default depth: Previously this was set low
			// to prevent run away globs but now we cancel
			// the query based on time so it really does
			// not matter.
			depth := 30

			// Allow the user to override the recursion depth.
			if len(groups[1]) > 0 {
				var err error
				if depth, err = strconv.Atoi(groups[1]); err != nil {
					return nil, err
				}
			}

			result = append(result, _RecursiveComponent{
				path: FNmatchTranslate(strings.Replace(
					path_component, groups[0], "*", 1)),
				depth: depth,
			})

		} else if m := _GLOB_MAGIC_CHECK.FindString(path_component); len(m) > 0 {
			result = append(result, &_RegexComponent{
				regexp: FNmatchTranslate(path_component),
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
func FNmatchTranslate(pat string) string {
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

func DevOf(file_info accessors.FileInfo) (uint64, bool) {
	dev, ok := file_info.(FileDev)
	if !ok {
		return 0, false
	}
	return dev.Dev(), true
}

// Duplicate brace expansions into multiple globs:
// /usr/bin/*.{exe,dll} -> /usr/bin/*.exe, /usr/bin/*.dll
func ExpandBraces(patterns []string) []string {
	result := make([]string, 0, len(patterns))

	for _, pattern := range patterns {
		_brace_expansion(pattern, &result)
	}

	return result
}

func _brace_expansion(pattern string, result *[]string) {
	groups := _GROUPING_PATTERN.FindStringSubmatch(pattern)
	if len(groups) > 0 {
		left := groups[1]
		middle := strings.Split(groups[2], ",")
		right := groups[3]

		for _, item := range middle {
			_brace_expansion(left+item+right, result)
		}
	} else if !utils.InString(*result, pattern) {
		*result = append(*result, pattern)
	}

}
