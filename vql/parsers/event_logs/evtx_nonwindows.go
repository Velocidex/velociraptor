// +build !windows

package event_logs

import "github.com/Velocidex/ordereddict"

func maybeEnrichEvent(event *ordereddict.Dict) *ordereddict.Dict {
	return event
}
