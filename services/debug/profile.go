package debug

import (
	"context"
	"strings"
	"sync"

	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type ProfileWriter func(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row)

type ProfileWriterInfo struct {
	Name, Description string
	ProfileWriter     ProfileWriter
	ID                uint64
	Categories        []string
}

var (
	mu       sync.Mutex
	handlers []ProfileWriterInfo
)

func RegisterProfileWriter(writer ProfileWriterInfo) {
	mu.Lock()
	defer mu.Unlock()

	handlers = append(handlers, writer)
}

func UnregisterProfileWriter(id uint64) {
	mu.Lock()
	defer mu.Unlock()

	new_handlers := make([]ProfileWriterInfo, 0, len(handlers))
	for _, h := range handlers {
		if h.ID != id {
			new_handlers = append(new_handlers, h)
		}
	}
	handlers = new_handlers
}

type CategoryTreeNode struct {
	// The current path of this node
	Path []string

	// A list of direct children.
	SubCategories map[string]*CategoryTreeNode

	// Profiles contained in this category.
	Profiles map[string]ProfileWriterInfo
}

func (self *CategoryTreeNode) toString() []string {
	res := []string{strings.Join(self.Path, "/")}

	for _, k := range utils.Sort(self.SubCategories) {
		sc := self.SubCategories[k]

		for _, l := range sc.toString() {
			res = append(res, "  "+l)
		}
	}

	for _, k := range utils.Sort(self.Profiles) {
		p, _ := self.Profiles[k]
		res = append(res, "- "+p.Name)
	}

	return res
}

func (self *CategoryTreeNode) String() string {
	return strings.Join(self.toString(), "\n")
}

func getChildren(self *CategoryTreeNode) {
	path_len := len(self.Path)

	for _, h := range handlers {
		// Direct descendant - add to current level.
		if utils.StringSliceEq(h.Categories, self.Path) {
			self.Profiles[h.Name] = h
			continue
		}

		// This node is above us in the tree - skip it
		if len(h.Categories) <= path_len {
			continue
		}

		// Node has the same prefix.
		if utils.StringSliceEq(self.Path, h.Categories[:path_len]) {

			// Name of the next level.
			name := h.Categories[path_len]
			next_path := append(self.Path, name)

			// Add as a SubCategory.
			// We already have it - skip it.
			_, pres := self.SubCategories[name]
			if pres {
				continue
			}

			new_node := &CategoryTreeNode{
				Path:          next_path,
				SubCategories: make(map[string]*CategoryTreeNode),
				Profiles:      make(map[string]ProfileWriterInfo),
			}
			self.SubCategories[name] = new_node
			getChildren(new_node)
		}
	}

}

func GetProfileTree() *CategoryTreeNode {
	mu.Lock()
	defer mu.Unlock()

	res := &CategoryTreeNode{
		SubCategories: make(map[string]*CategoryTreeNode),
	}
	getChildren(res)
	return res
}

func GetProfileWriters() (result []ProfileWriterInfo) {
	mu.Lock()
	defer mu.Unlock()

	for _, i := range handlers {
		result = append(result, i)
	}

	return result
}
