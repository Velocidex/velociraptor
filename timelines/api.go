package timelines

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/vfilter"
)

// Writes time series data to storage. Assumes data is written in
// increasing time order.
type ITimelineWriter interface {
	Stats() *timelines_proto.Timeline
	Write(timestamp time.Time, row *ordereddict.Dict) error
	WriteBuffer(timestamp time.Time, serialized []byte) error
	Truncate()
	Close()
}

type ITimelineReader interface {
	Stat() *timelines_proto.Timeline
	SeekToTime(timestamp time.Time) error
	Read(ctx context.Context) <-chan TimelineItem
	Close()
	New(config_obj *config_proto.Config,
		ransformer Transformer,
		path_manager paths.TimelinePathManagerInterface) (*TimelineReader, error)
}

// Reads a super timeline. Super timelines include multiple component
// timelines. This reader allows them to be switched on and off only
// reading the data from the selected set.
type ISuperTimelineReader interface {
	Stat() *timelines_proto.SuperTimeline
	Close()
	SeekToTime(timestamp time.Time)
	Read(ctx context.Context) <-chan TimelineItem
	New(ctx context.Context,
		config_obj *config_proto.Config,
		storer ISuperTimelineStorer,
		notebook_id, super_timeline string,
		include_components []string, exclude_components []string) (ISuperTimelineReader, error)
}

// Writes time series data into a Super Timeline. Allows adding or
// replacing a component from the timeline.
type ISuperTimelineWriter interface {
	Close(ctx context.Context) error

	// Adds a new component to the timeline. If the component is
	// already there, remove the old data and replace it with existing
	// data.
	AddChild(timeline *timelines_proto.Timeline, completer func()) (ITimelineWriter, error)
	New(ctx context.Context,
		config_obj *config_proto.Config,
		storer ISuperTimelineStorer,
		notebook_id, name string) (ISuperTimelineWriter, error)
}

// Stores metadata about the super timeline.
type ISuperTimelineStorer interface {
	Get(ctx context.Context, notebook_id string,
		name string) (*timelines_proto.SuperTimeline, error)
	Set(ctx context.Context, notebook_id string,
		timeline *timelines_proto.SuperTimeline) error

	GetTimeline(ctx context.Context, notebook_id string,
		super_timeline, component string) (*timelines_proto.Timeline, error)

	UpdateTimeline(ctx context.Context,
		notebook_id string, supertimeline string,
		timeline *timelines_proto.Timeline) (*timelines_proto.SuperTimeline, error)

	DeleteComponent(ctx context.Context, notebook_id string,
		super_timeline, component string) error

	List(ctx context.Context,
		notebook_id string) ([]*timelines_proto.SuperTimeline, error)

	GetAvailableTimelines(ctx context.Context, notebook_id string) []string
}

// Adds an annotation to the super timeline. This is normally a
// separate component called "Annotations"
type ISuperTimelineAnnotator interface {
	AnnotateTimeline(
		ctx context.Context, scope vfilter.Scope,
		notebook_id string, supertimeline string,
		message, principal string,
		timestamp time.Time, event *ordereddict.Dict) error
}
