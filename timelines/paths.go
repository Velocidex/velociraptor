package timelines

import (
	"path"
)

type TimelinePathManagerInterface interface {
	Path() string
	Index() string
	Name() string
}

type TimelinePathManager struct {
	name string
	root string
}

func (self TimelinePathManager) Path() string {
	return self.root + "/" + self.name + ".json"
}

func (self TimelinePathManager) Name() string {
	return self.name
}

func (self TimelinePathManager) Index() string {
	return self.root + "/" + self.name + ".idx"
}

// A Supertimeline is a collection of individual timelines. Create
// this path manager using a notebook path manager.
type SuperTimelinePathManager struct {
	Name string
	Root string // Base directory where we store the timeline.
}

func (self *SuperTimelinePathManager) Path() string {
	return path.Join("/", self.Root, "timelines", self.Name+".json")
}

// Add a child timeline to the super timeline.
func (self *SuperTimelinePathManager) GetChild(child_name string) *TimelinePathManager {
	return &TimelinePathManager{
		name: child_name,
		root: path.Join("/", self.Root, "timelines", self.Name),
	}
}
