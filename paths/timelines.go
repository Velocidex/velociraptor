package paths

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type TimelinePathManagerInterface interface {
	Path() api.FSPathSpec
	Index() api.FSPathSpec
	Name() string
}

type TimelinePathManager struct {
	name string
	root api.FSPathSpec
}

func (self TimelinePathManager) Path() api.FSPathSpec {
	return self.root
}

func (self TimelinePathManager) Name() string {
	return self.name
}

func (self TimelinePathManager) Index() api.FSPathSpec {
	return self.root.SetType(api.PATH_TYPE_FILESTORE_JSON_TIME_INDEX)
}

func NewTimelinePathManager(name string, root api.FSPathSpec) *TimelinePathManager {
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
	Root api.DSPathSpec
}

func (self *SuperTimelinePathManager) Path() api.DSPathSpec {
	return self.Root.AddUnsafeChild(self.Name)
}

// Add a child timeline to the super timeline.
func (self *SuperTimelinePathManager) GetChild(
	child_name string) *TimelinePathManager {
	return &TimelinePathManager{
		name: child_name,
		root: self.Root.AddUnsafeChild(self.Name, child_name).
			AsFilestorePath(),
	}
}
