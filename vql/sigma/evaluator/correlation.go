/*
  Sigma correlations are rules that track the output of regular rules
  inside a correlation context. The correlation context maintains
  state about previous matches so that additional checks can be made
  across rules.

  Full details here https://sigmahq.io/docs/meta/correlations.html

  ## Velociraptor's implementation:

  Velociraptor designates a correlation using the `SigmaCorrelator`
  type. This type references a number of rules by their names. The
  Sigma compiler will insert a reference to the `SigmaCorrelator`
  instance in all these rules so that when they file, the event is
  also relayed to the `SigmaCorrelator` object.

  Correlations are typically grouped by a set of fields. The
  `SigmaCorrelator` keeps a separate `SigmaCorrelatorGroup` object to
  track each group separately.

  Once the correct `SigmaCorrelatorGroup` object is found, we add the
  event to the `timespanManager`. This object is responsible for
  maintaining a sliding time window of all matching events within the
  specified `timespan`. If the correlation rule does not specify a
  timespan we use 5 minutes.

  The `timespanManager` is responsible for adding the new event, as
  well as evicting events that fell out of the timespan. As this
  happens the `correlationComparator` is informed of events that are
  added or removed from the timespan.

  There are a number of different "flavors" of correlations
  implemented by different `correlationComparator` instances. Each
  `correlationComparator` maintains running state:

  1. eventCount: This object maintains the total count of events in
     the timespan.

  2. valueCount: This object maintains the total count of unique field
     values within the timespan. The condition clause must contain a
     `field` value.

  3. temporal: This object maintains the total rules that match.

*/

package evaluator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/sigma-go"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

// A correlationComparator maintains state about a single group.
type correlationComparator interface {
	// The timespan manager adds and evicts events from the timespan
	// and communicates this to the correlationComparator
	addEvent(ctx context.Context, scope types.Scope,
		event *TimedEvent, rule *VQLRuleEvaluator)

	evictEvent(ctx context.Context, scope types.Scope,
		event *TimedEvent, rule *VQLRuleEvaluator)

	// Check the current state to see if a match is present.
	check() bool
}

type eventCount struct {
	total_events int

	cmp func(count int) bool
}

func (self *eventCount) addEvent(
	ctx context.Context, scope types.Scope,
	event *TimedEvent, rule *VQLRuleEvaluator) {
	self.total_events++
}

func (self *eventCount) evictEvent(
	ctx context.Context, scope types.Scope,
	event *TimedEvent, rule *VQLRuleEvaluator) {
	self.total_events--
}

func (self *eventCount) check() bool {
	return self.cmp(self.total_events)
}

func NewEventCount(
	ctx context.Context, scope types.Scope,
	rule sigma.Rule) (*eventCount, error) {
	condition := rule.Correlation.Condition
	cmp, err := getCmp(scope, condition)
	if err != nil {
		return nil, fmt.Errorf("While parsing rule %v: %w", rule.Title, err)
	}

	return &eventCount{
		cmp: cmp,
	}, nil
}

type valueCount struct {
	value_field string
	value_map   map[string]int

	cmp func(count int) bool
}

func NewValueCount(
	ctx context.Context, scope types.Scope,
	rule sigma.Rule) (*valueCount, error) {
	condition := rule.Correlation.Condition

	if condition == nil {
		return nil, fmt.Errorf("While parsing rule %v: value_count rule requires a condition", rule.Title)
	}
	value_field_any, pres := condition["field"]
	if !pres {
		return nil, fmt.Errorf("While parsing rule %v: value_count rule requires a field in condition clause", rule.Title)
	}

	cmp, err := getCmp(scope, condition)
	if err != nil {
		return nil, fmt.Errorf("While parsing rule %v: %w", rule.Title, err)
	}

	return &valueCount{
		value_map:   make(map[string]int),
		value_field: utils.ToString(value_field_any),
		cmp:         cmp,
	}, nil
}

func (self *valueCount) check() bool {
	return self.cmp(len(self.value_map))
}

func (self *valueCount) addEvent(
	ctx context.Context, scope types.Scope,
	event *TimedEvent, rule *VQLRuleEvaluator) {
	field_any, err := rule.GetFieldValuesFromEvent(
		ctx, scope, self.value_field, event.Event)
	if err != nil || len(field_any) == 0 {
		return
	}
	field := utils.ToString(field_any[0])

	count := self.value_map[field]
	self.value_map[field] = count + 1
}

func (self *valueCount) evictEvent(
	ctx context.Context, scope types.Scope,
	event *TimedEvent, rule *VQLRuleEvaluator) {
	field_any, err := rule.GetFieldValuesFromEvent(
		ctx, scope, self.value_field, event.Event)
	if err != nil || len(field_any) == 0 {
		return
	}
	field := utils.ToString(field_any[0])

	count := self.value_map[field]
	self.value_map[field] = count - 1
	if count-1 <= 0 {
		delete(self.value_map, field)
	}
}

type temporal struct {
	value_map       map[string]int
	number_of_rules int
}

func NewTemporal(
	ctx context.Context, scope types.Scope,
	rule sigma.Rule) (*temporal, error) {
	return &temporal{
		value_map:       make(map[string]int),
		number_of_rules: len(rule.Correlation.Rules),
	}, nil
}

func (self *temporal) check() bool {
	return len(self.value_map) >= self.number_of_rules
}

func (self *temporal) addEvent(
	ctx context.Context, scope types.Scope,
	event *TimedEvent, rule *VQLRuleEvaluator) {
	name := rule.Name
	if name == "" {
		name = rule.ID
	}

	count := self.value_map[name]
	self.value_map[name] = count + 1
}

func (self *temporal) evictEvent(
	ctx context.Context, scope types.Scope,
	event *TimedEvent, rule *VQLRuleEvaluator) {
	name := rule.Name
	if name == "" {
		name = rule.ID
	}

	count := self.value_map[name]
	self.value_map[name] = count - 1
	if count-1 <= 0 {
		delete(self.value_map, name)
	}
}

type timespanManager struct {
	timespan time.Duration
	times    []*TimedEvent

	correlator correlationComparator
}

func NewTimespanManager(
	correlator correlationComparator,
	rule sigma.Rule) (*timespanManager, error) {
	timespan_str := "5m"
	if rule.Correlation.Timespan != "" {
		timespan_str = rule.Correlation.Timespan
	}

	timespan, err := time.ParseDuration(timespan_str)
	if err != nil {
		return nil, fmt.Errorf("While parsing rule timespan for %v: %w",
			rule.Title, err)
	}

	return &timespanManager{
		timespan:   timespan,
		correlator: correlator,
	}, nil
}

// Add a new event to the timespan
func (self *timespanManager) addTime(
	ctx context.Context, scope types.Scope,
	event *Event, rule *VQLRuleEvaluator) error {

	timestamp_any, err := rule.GetFieldValuesFromEvent(
		ctx, scope, "Timestamp", event)
	if err != nil {
		return err
	}

	if len(timestamp_any) == 0 {
		return fmt.Errorf("Unable to parse timestamp")
	}

	ts, err := functions.TimeFromAny(ctx, scope, timestamp_any[0])
	if err != nil {
		return err
	}

	new_event := &TimedEvent{
		ts:    ts,
		Event: event,
	}
	self.correlator.addEvent(ctx, scope, new_event, rule)

	self.times = append(self.times, new_event)

	latest := self.times[len(self.times)-1]

	var new_times []*TimedEvent

	// The earliest time we can accept, expire times that are too
	// old.
	earliest := latest.ts.Add(-self.timespan)
	for _, t := range self.times {
		if t.ts.After(earliest) {
			new_times = append(new_times, t)
		} else {
			self.correlator.evictEvent(ctx, scope, t, rule)
		}
	}
	self.times = new_times

	return nil
}

func (self *timespanManager) getEvents() []*Event {
	// Only bother to sort when we actually have a hit
	sort.Slice(self.times, func(i, j int) bool {
		return self.times[i].ts.Unix() < self.times[j].ts.Unix()
	})
	events := make([]*Event, 0, len(self.times))
	for _, e := range self.times {
		events = append(events, e.Event)
	}
	return events
}

type TimedEvent struct {
	ts    time.Time
	Event *Event `json:",inline" yaml:",inline"`
}

// Each group in the correlator contains its own context.
type SigmaCorrelatorGroup struct {
	timespanManager *timespanManager
	correlator      correlationComparator
}

// Matches the new event against the correlation context
func (self *SigmaCorrelatorGroup) Match(
	ctx context.Context, scope types.Scope,
	event *Event,
	rule *VQLRuleEvaluator) (*Result, error) {

	err := self.timespanManager.addTime(ctx, scope, event, rule)
	if err != nil {
		return nil, err
	}

	if self.correlator.check() {
		return &Result{
			Match:           true,
			CorrelationHits: self.timespanManager.getEvents(),
		}, nil

	} else {
		// Suppress the hit
		return &Result{
			Match:           false,
			CorrelationHits: nil,
		}, nil
	}
}

// An object that manages a single group within a sigma correlation
// rule.
func NewSigmaCorrelatorGroup(
	ctx context.Context, scope types.Scope,
	rule sigma.Rule) (*SigmaCorrelatorGroup, error) {

	// Figure out the type of the correlation
	switch rule.Correlation.Type {
	case "event_count":
		correlator, err := NewEventCount(ctx, scope, rule)
		if err != nil {
			return nil, err
		}

		ts, err := NewTimespanManager(correlator, rule)
		if err != nil {
			return nil, err
		}

		return &SigmaCorrelatorGroup{
			timespanManager: ts,
			correlator:      correlator,
		}, nil

	case "value_count":
		correlator, err := NewValueCount(ctx, scope, rule)
		if err != nil {
			return nil, err
		}

		ts, err := NewTimespanManager(correlator, rule)
		if err != nil {
			return nil, err
		}

		return &SigmaCorrelatorGroup{
			timespanManager: ts,
			correlator:      correlator,
		}, nil

	case "temporal":
		correlator, err := NewTemporal(ctx, scope, rule)
		if err != nil {
			return nil, err
		}

		ts, err := NewTimespanManager(correlator, rule)
		if err != nil {
			return nil, err
		}

		return &SigmaCorrelatorGroup{
			timespanManager: ts,
			correlator:      correlator,
		}, nil

	//case "ordered_temporal":
	default:
		return nil, fmt.Errorf("Unsupported correlation type for %v: %v",
			rule.Title, rule.Correlation.Type)
	}
}

// The comparison operators allowed in a correlation condition clause.
var cmpOps = map[string]func(scope vfilter.Scope, count int, value interface{}) bool{
	"gt": func(scope vfilter.Scope, count int, value interface{}) bool {
		return scope.Gt(count, value)
	},
	"gte": func(scope vfilter.Scope, count int, value interface{}) bool {
		return scope.Gt(count, value) || scope.Eq(count, value)
	},
	"lt": func(scope vfilter.Scope, count int, value interface{}) bool {
		return scope.Lt(count, value)
	},
	"lte": func(scope vfilter.Scope, count int, value interface{}) bool {
		return scope.Lt(count, value) || scope.Eq(count, value)
	},
	"eq": func(scope vfilter.Scope, count int, value interface{}) bool {
		return scope.Eq(count, value)
	},
	"neq": func(scope vfilter.Scope, count int, value interface{}) bool {
		return !scope.Eq(count, value)
	},
}

// Build a comparator from the condition clause. All operators
// present must pass, so several operators together form a range
// (e.g. gte: 5, lte: 10). Unknown operators are rejected so a typo
// does not silently match everything.
func getCmp(scope vfilter.Scope,
	condition map[string]interface{}) (func(count int) bool, error) {

	type check struct {
		op    func(scope vfilter.Scope, count int, value interface{}) bool
		value interface{}
	}

	var checks []check
	for k, v := range condition {
		// value_count names the field to count here.
		if k == "field" {
			continue
		}

		op, pres := cmpOps[k]
		if !pres {
			return nil, fmt.Errorf("unsupported correlation condition %q", k)
		}
		checks = append(checks, check{op: op, value: v})
	}

	return func(count int) bool {
		for _, c := range checks {
			if !c.op(scope, count, c.value) {
				return false
			}
		}
		return true
	}, nil
}

// One correlator per correlation rule
type SigmaCorrelator struct {
	*VQLRuleEvaluator

	mu     sync.Mutex
	lookup map[string]*SigmaCorrelatorGroup
}

func (self *SigmaCorrelator) getGroupKey(
	ctx context.Context, scope types.Scope,
	rule *VQLRuleEvaluator,
	event *Event) string {
	var parts []string
	for _, gb := range self.Correlation.GroupBy {
		values, err := rule.GetFieldValuesFromEvent(ctx, scope, gb, event)
		if err != nil {
			continue
		}

		for _, v := range values {
			parts = append(parts, utils.ToString(v))
		}
	}

	return strings.Join(parts, "\x00")
}

func (self *SigmaCorrelator) Match(
	ctx context.Context, scope types.Scope,
	rule *VQLRuleEvaluator,
	event *Event) (*Result, error) {
	var err error

	self.mu.Lock()
	defer self.mu.Unlock()

	// Get the group by matcher
	gb_key := self.getGroupKey(ctx, scope, rule, event)
	gb_matcher, pres := self.lookup[gb_key]
	if pres {
		return gb_matcher.Match(ctx, scope, event, rule)
	}

	gb_matcher, err = NewSigmaCorrelatorGroup(ctx, scope, self.Rule)
	if err != nil {
		return nil, err
	}
	self.lookup[gb_key] = gb_matcher
	return gb_matcher.Match(ctx, scope, event, rule)
}

// One correlator per rule.
func NewSigmaCorrelator(
	scope types.Scope,
	rule sigma.Rule,
	fieldmappings *FieldMappingResolver) (*SigmaCorrelator, error) {
	evaluator := NewVQLRuleEvaluator(scope, rule, fieldmappings)
	err := evaluator.CheckRule()
	if err != nil {
		return nil, err
	}

	return &SigmaCorrelator{
		VQLRuleEvaluator: evaluator,
		lookup:           make(map[string]*SigmaCorrelatorGroup),
	}, nil
}
