package timelines

import (
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
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

func (self timelineTransformer) buildDefaultMessage(event *ordereddict.Dict) string {
	var b strings.Builder
	for _, item := range event.Items() {
		// Drop the timestamp as it is already included
		if item.Key == self.TimestampColumn {
			continue
		}

		b.WriteString(item.Key)
		b.WriteString(": ")
		b.WriteString(utils.ToString(item.Value))
		b.WriteString(" ")

		// Truncate to 80 chars
		if b.Len() > 80 {
			return b.String()[:80] + " ..."
		}
	}
	return b.String()
}

func (self timelineTransformer) Transform(
	source string,
	timestamp time.Time, event *ordereddict.Dict) TimelineItem {

	// Extract some standard fields
	message_column := self.MessageColumn
	if message_column == "" {
		message_column = "Message"
	}

	var message string
	message_any, pres := event.Get(message_column)
	if pres {
		message = utils.ToString(message_any)
	} else {
		message = self.buildDefaultMessage(event)
	}

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
