package timelines

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type TimelinePathManagerInterface interface {
	Path() api.PathSpec
	Index() api.PathSpec
	Name() string
}

type TimelinePathManager struct {
	name string
	root api.PathSpec
}

func (self TimelinePathManager) Path() api.PathSpec {
	return self.root.AddChild(self.name).SetType("json")
}

func (self TimelinePathManager) Name() string {
	return self.name
}

func (self TimelinePathManager) Index() api.PathSpec {
	return self.root.AddChild(self.name).SetType("json.idx")
}

// A Supertimeline is a collection of individual timelines. Create
// this path manager using a notebook path manager.
type SuperTimelinePathManager struct {
	Name string

	// Base directory where we store the timeline.
	Root api.PathSpec
}

func (self *SuperTimelinePathManager) Path() api.PathSpec {
	return self.Root.AddChild(self.Name)
}

// Add a child timeline to the super timeline.
func (self *SuperTimelinePathManager) GetChild(
	child_name string) *TimelinePathManager {
	return &TimelinePathManager{
		name: child_name,
		root: self.Root.AddChild("timelines", self.Name),
	}
}
