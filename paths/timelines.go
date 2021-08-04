package paths

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
	return self.root.AddChild(self.name)
}

func (self TimelinePathManager) Name() string {
	return self.name
}

func (self TimelinePathManager) Index() api.PathSpec {
	return self.root.AddChild(self.name).
		SetType(api.PATH_TYPE_FILESTORE_JSON_TIME_INDEX)
}

func NewTimelinePathManager(name string, root api.PathSpec) *TimelinePathManager {
	return &TimelinePathManager{
		name: name,
		root: root,
	}
}

// A Supertimeline is a collection of individual timelines. Create
// this path manager using a notebook path manager.
type SuperTimelinePathManager struct {
	Name string

	// Base directory where we store the timeline.
	Root api.PathSpec
}

func NewSuperTimelinePathManager(
	name string, root api.PathSpec) *SuperTimelinePathManager {
	return &SuperTimelinePathManager{
		Name: name,
		Root: root,
	}
}

func (self *SuperTimelinePathManager) Path() api.PathSpec {
	return self.Root.AddUnsafeChild(self.Name)
}

// Add a child timeline to the super timeline.
func (self *SuperTimelinePathManager) GetChild(
	child_name string) *TimelinePathManager {
	return &TimelinePathManager{
		name: child_name,
		root: self.Root.AddUnsafeChild(self.Name).
			SetType(api.PATH_TYPE_FILESTORE_JSON),
	}
}
