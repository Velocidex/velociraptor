package timelines

import (
	"fmt"
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
	message_column := self.MessageColumn
	if message_column == "" {
		message_column = "Message"
	}
	message_any, _ := event.Get(message_column)
	message := toStr(message_any)

	timestamp_description_column := self.TimestampDescriptionColumn
	if timestamp_description_column == "" {
		timestamp_description_column = "Description"
	}
	timestamp_description, _ := event.GetString(timestamp_description_column)

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

func toStr(in interface{}) string {
	s, ok := in.(string)
	if ok {
		return s
	}

	return fmt.Sprintf("%v", in)
}
