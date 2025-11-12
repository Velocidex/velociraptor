package utils

import (
	"fmt"
	"strings"
)

const (
	CaseInsensitivePrefix = true
)

// A prefix tree is used to quickly look up array prefixes.
type PrefixNode struct {
	Name            string
	Depth           int
	Sentinel        bool
	Children        map[string]*PrefixNode
	CaseInsensitive bool
}

func (self *PrefixNode) DebugString() string {
	if self.Sentinel {
		return "*"
	}

	var child_dbg []string
	for _, v := range self.Children {
		child_dbg = append(child_dbg, v.DebugString())
	}
	return fmt.Sprintf("%v: %v", self.Name, child_dbg)
}

func (self *PrefixNode) ToLower(in string) string {
	if self.CaseInsensitive {
		return strings.ToLower(in)
	}
	return in
}

func (self *PrefixNode) Add(components []string) {
	if len(components) == 0 {
		self.Sentinel = true
		return
	}

	first := self.ToLower(components[0])
	child, pres := self.Children[first]
	if !pres {
		child = NewPrefixNode(first, self.CaseInsensitive, self.Depth+1)
		self.Children[first] = child
	}

	child.Add(components[1:])
}

func (self *PrefixNode) Present(components []string) (bool, int) {
	if len(components) == 0 {
		// Perfect match - the tested component is the same as this.
		if self.Sentinel {
			return true, self.Depth
		}
		// The tested path is shorter than this level.
		return false, 0
	}

	first := self.ToLower(components[0])
	child, pres := self.Children[first]
	if !pres {
		return self.Sentinel, self.Depth
	}

	// Depth first search to find the logest matching prefix
	match, depth := child.Present(components[1:])
	if match {
		return match, depth
	}

	// We only found a match if this is also a sentinel.
	return self.Sentinel, self.Depth
}

func NewPrefixNode(name string, case_insensitive bool, depth int) *PrefixNode {
	return &PrefixNode{
		Name:            name,
		Depth:           depth,
		Children:        make(map[string]*PrefixNode),
		CaseInsensitive: case_insensitive,
	}
}

type PrefixTree struct {
	root *PrefixNode
}

func NewPrefixTree(case_insensitive bool) *PrefixTree {
	return &PrefixTree{
		root: NewPrefixNode("", case_insensitive, 0),
	}
}

func (self *PrefixTree) Add(components []string) {
	self.root.Add(components)
}

// Returns if the path matches any prefix in the prefix tree, as well
// as the depth of the tree at which a match is made.
func (self *PrefixTree) Present(components []string) (bool, int) {
	return self.root.Present(components)
}

func (self *PrefixTree) DebugString() string {
	return self.root.DebugString()
}
