package timelines

import (
	"time"

	"github.com/Velocidex/ordereddict"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
)

const (
	UnitTransformer = unitTransformer(0)
)

// Transforms the raw event that is written in the result set to a
// TimelineItem
type Transformer interface {
	Transform(
		source string,
		timestamp time.Time,
		event *ordereddict.Dict) TimelineItem
}

type unitTransformer int

func (self unitTransformer) Transform(
	source string,
	timestamp time.Time, event *ordereddict.Dict) TimelineItem {
	return TimelineItem{
		Source: source,
		Row:    event,
		Time:   timestamp,
	}
}

type timelineTransformer struct {
	*timelines_proto.Timeline
}

func (self timelineTransformer) Transform(
	source string,
	timestamp time.Time, event *ordereddict.Dict) TimelineItem {

	// Extract some standard fields
	message, _ := event.GetString(self.MessageColumn)
	timestamp_description, _ := event.GetString(self.TimestampDescriptionColumn)

	event.Delete(self.MessageColumn)
	event.Delete(self.TimestampDescriptionColumn)

	return TimelineItem{
		Row:                  event,
		Time:                 timestamp,
		Message:              message,
		TimestampDescription: timestamp_description,
		Source:               source,
	}
}
