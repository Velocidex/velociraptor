package utils

import (
	"fmt"
	"strings"
)

// A prefix tree is used to quickly look up array prefixes.
type PrefixNode struct {
	Name            string
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
		child = NewPrefixNode(first, self.CaseInsensitive)
		self.Children[first] = child
	}

	child.Add(components[1:])
}

func (self *PrefixNode) Present(components []string) bool {
	if len(components) == 0 || self.Sentinel {
		return true
	}

	first := self.ToLower(components[0])
	child, pres := self.Children[first]
	if !pres {
		return false
	}

	return child.Present(components[1:])
}

func NewPrefixNode(name string, case_insensitive bool) *PrefixNode {
	return &PrefixNode{
		Name:            name,
		Children:        make(map[string]*PrefixNode),
		CaseInsensitive: case_insensitive,
	}
}

type PrefixTree struct {
	root *PrefixNode
}

func NewPrefixTree(case_insensitive bool) *PrefixTree {
	return &PrefixTree{
		root: NewPrefixNode("", case_insensitive),
	}
}

func (self *PrefixTree) Add(components []string) {
	self.root.Add(components)
}

func (self *PrefixTree) Present(components []string) bool {
	return self.root.Present(components)
}

func (self *PrefixTree) DebugString() string {
	return self.root.DebugString()
}
