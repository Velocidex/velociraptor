// +build windows

package event_logs

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/evtx"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
)

var (
	lru *cache.LRUCache = cache.NewLRUCache(1000)
)

type cachedMessageSet struct {
	*evtx.MessageSet
}

func (self cachedMessageSet) Size() int {
	return 1
}

// maybeEnrichEvent searches for the event messages in the system's
// event providers.
func maybeEnrichEvent(event *ordereddict.Dict) *ordereddict.Dict {
	// Event.System.Provider.Name and Event.System.Provider.Guid
	// are alternate ways of specifying the provider. Some
	// providers have either one or both so we allow either one to
	// be blank here.
	provider, _ := ordereddict.GetString(event, "System.Provider.Name")
	provider_guid, _ := ordereddict.GetString(event, "System.Provider.Guid")

	channel, ok := ordereddict.GetString(event, "System.Channel")
	if !ok {
		return event
	}

	event_id, ok := ordereddict.GetInt(event, "System.EventID.Value")
	if !ok {
		return event
	}

	key := channel + provider + provider_guid
	var message_set *evtx.MessageSet
	var err error

	cached_message_set, pres := lru.Get(key)
	if !pres {
		message_set, err = evtx.GetMessagesByGUID(provider_guid, channel)
		if err != nil {
			message_set, err = evtx.GetMessages(provider, channel)
			if err != nil {
				return event
			}
		}

		lru.Set(key, cachedMessageSet{message_set})
	} else {
		message_set = cached_message_set.(cachedMessageSet).MessageSet
	}

	msg, pres := message_set.Messages[event_id]
	if pres {
		event.Set("Message", evtx.ExpandMessage(event, msg.Message))
	}

	return event
}
