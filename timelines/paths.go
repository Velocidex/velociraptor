package timelines

import (
	"path"
)

type TimelinePathManager struct {
	Name string
	root string
}

func (self TimelinePathManager) Path() string {
	return self.root + "/" + self.Name + ".json"
}

func (self TimelinePathManager) Index() string {
	return self.root + "/" + self.Name + ".idx"
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
		Name: child_name,
		root: self.Path(),
	}
}
