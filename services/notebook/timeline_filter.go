package notebook

import (
	"regexp"

	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/timelines"
)

type TimelineFilter struct {
	regex *regexp.Regexp
}

func (self *TimelineFilter) ShouldFilter(event *timelines.TimelineItem) bool {
	if self.regex == nil || event.Row == nil {
		return false
	}

	// Match the message first as an optimization.
	if self.regex.MatchString(event.Message) {
		return false
	}

	// We try to match any of the additional event data.
	serialized, err := event.Row.MarshalJSON()
	if err != nil {
		return false
	}

	// Filter the row out if we do not match anywhere in the event. We
	// only want matching events to go through.
	if !self.regex.MatchString(string(serialized)) {
		return true
	}

	return false
}

func NewTimelineFilter(options services.TimelineOptions) (
	result *TimelineFilter, err error) {
	result = &TimelineFilter{}
	if options.Filter != "" {
		result.regex, err = regexp.Compile("(?i)" + options.Filter)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}
