package evaluator

import (
	"context"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

// Wrap the row in an Event object which caches lambda lookups.

// Many sigma rules share similar detections. For example many rules
// start by matching on the Channel or the event ID. Since VQL is an
// interpreted language performing the field mapping operation results
// in calling the field mapping lambda which can slow things down.

// Since we are checking the same event against many rules, it is safe
// to assume that the field mapping lambda is invarient with respect
// to the rules. Therefore we can cache it between rule evaluation.

// This significantly speeds up matching since we avoid calling the
// lambda for each rule and instead call it once for the first rule to
// use this field.
type Event struct {
	// This is the original event from the log source.
	*ordereddict.Dict

	// This caches the sigma fields which are reduced by the sigma
	// field mapping lambdas. The same event is passed through the
	// entire rule chain so this caching avoids calculating the sigma
	// fields multiple times.
	mu         sync.Mutex
	cache      map[string]types.Any
	cache_json string
}

func (self *Event) AsJson() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.cache_json == "" {
		serialized, _ := self.Dict.MarshalJSON()
		self.cache_json = strings.ToLower(string(serialized))
	}

	return self.cache_json
}

func (self *Event) Copy() *ordereddict.Dict {
	result := ordereddict.NewDict()
	result.MergeFrom(self.Dict)
	return result
}

func (self *Event) Reduce(
	ctx context.Context, scope types.Scope,
	field string, lambda *vfilter.Lambda) types.Any {
	self.mu.Lock()
	defer self.mu.Unlock()

	cached, pres := self.cache[field]
	if pres {
		return cached
	}

	// Call the lambda for the real value
	cached = lambda.Reduce(ctx, scope, []types.Any{self.Dict})
	self.cache[field] = cached

	return cached
}

func NewEvent(event *ordereddict.Dict) *Event {
	copy := ordereddict.NewDict()
	copy.MergeFrom(event)

	return &Event{
		Dict:  copy,
		cache: make(map[string]types.Any),
	}
}
