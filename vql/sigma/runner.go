package sigma

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/sigma-go"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

// Each log source combines rules and consumes a single event stream.
type SigmaExecutionContext struct {
	eval_time   int64
	event_count uint64

	Name string

	query types.StoredQuery

	// Rules not using correlations - can be run in any order.
	rules []*evaluator.VQLRuleEvaluator

	// Rules consumed by correlations - must be run in order
	correlations     []*evaluator.VQLRuleEvaluator
	non_correlations []*evaluator.VQLRuleEvaluator

	active bool
}

func (self *SigmaExecutionContext) ChargeTime(ns int64) {
	atomic.AddInt64(&self.eval_time, ns)
}

// Break down the rules into two separate lists - correlations are run
// in serial while non-correlations run in parallel
func (self *SigmaExecutionContext) balance() {
	for _, r := range self.rules {
		if r.Correlator == nil {
			self.non_correlations = append(self.non_correlations, r)
		} else {
			self.correlations = append(self.correlations, r)
		}
	}
}

// Start the evaluation loop - start the query and consume events from it.
func (self *SigmaExecutionContext) Start(
	ctx context.Context, scope vfilter.Scope, output_chan chan vfilter.Row,
	pool *workerPool, wg *sync.WaitGroup) {

	defer wg.Done()

	subscope := scope.Copy()
	self.balance()

	defer subscope.Close()

	start := utils.GetTime().Now()

	self.active = true
	defer func() {
		scope.Log("INFO:sigma: Consumed %v messages from log source %v on %v rules (%v)",
			atomic.LoadUint64(&self.event_count), self.Name, len(self.rules),
			utils.GetTime().Now().Sub(start))
		self.active = false
	}()

	query_log := actions.QueryLog.AddQuery(
		vfilter.FormatToString(subscope, self.query))
	defer query_log.Close()

	for row := range self.query.Eval(ctx, subscope) {
		atomic.AddUint64(&self.event_count, 1)

		row_dict := toDict(subscope, row)

		// Evalute the row with all relevant
		// rules. Correlations are evaluted inline because
		// they need to be ordered.
		if len(self.correlations) > 0 {
			pool.RunInline(self, ctx, subscope,
				evaluator.NewEvent(row_dict), self.correlations)
		}

		// Evaluate the rest of the rules in parallel.
		if len(self.non_correlations) > 0 {
			pool.Run(self, ctx, subscope,
				evaluator.NewEvent(row_dict),
				self.non_correlations)
		}
	}
}

func (self *SigmaExecutionContext) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	for _, rule := range self.rules {
		if !self.active {
			continue
		}

		eval_time := time.Duration(atomic.LoadInt64(&self.eval_time)).
			Round(time.Millisecond).String()

		stats := ordereddict.NewDict().
			Set("LogSource", self.Name).
			Set("Rules", len(self.rules)).
			Set("EvalTime", eval_time).
			Set("Events", atomic.LoadUint64(&self.event_count))

		output_chan <- rule.Stats(stats)
	}
}

type SigmaContext struct {
	runners []*SigmaExecutionContext

	// Map between sigma field names to event. The lambda will be
	// passed the event. For example EID can be the lambda
	// x=>x.System.EventID.Value
	fieldmappings *evaluator.FieldMappingResolver

	mu          sync.Mutex
	debug       bool
	total_rules int
	hit_count   int
	id          uint64

	pool *workerPool

	output_chan chan types.Row
	wg          sync.WaitGroup

	default_details *vfilter.Lambda
}

func (self *SigmaContext) GetHitCount() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.hit_count
}

func (self *SigmaContext) IncHitCount() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.hit_count++
}

func (self *SigmaContext) Rows(
	ctx context.Context, scope types.Scope) chan vfilter.Row {

	// Start all the log sources now.
	for _, runner := range self.runners {
		self.wg.Add(1)
		go runner.Start(ctx, scope, self.output_chan, self.pool, &self.wg)
	}

	go func() {
		self.wg.Wait()

		// Close the channel once all the log sources are done.
		close(self.output_chan)
	}()

	return self.output_chan
}

func (self *SigmaContext) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {
	for _, runner := range self.runners {
		runner.ProfileWriter(ctx, scope, output_chan)
	}
}

func (self *SigmaContext) Close() {
	Tracker.Unregister(self)
}

func NewSigmaContext(
	ctx context.Context,
	scope types.Scope,
	rules []sigma.Rule,
	fieldmappings *ordereddict.Dict,
	log_sources *LogSourceProvider,
	default_details *vfilter.Lambda,
	debug bool) (*SigmaContext, error) {

	output_chan := make(chan vfilter.Row)

	rules_by_name := make(map[string]*evaluator.VQLRuleEvaluator)

	field_mapping_resolver := evaluator.NewFieldMappingResolver()

	self := &SigmaContext{
		output_chan:     output_chan,
		default_details: default_details,
		debug:           debug,
		fieldmappings:   field_mapping_resolver,
		id:              utils.GetId(),
	}

	// Compile the field mappings.  NOTE: The compiled fieldmappings
	// is shared between all the worker goroutines. Benchmarking shows
	// it is faster to make a slice copy than having to use a mutex to
	// protect it. This is O(1) but lock free. Using map copies uses
	// up significant amount of memory for local map copies.
	if fieldmappings != nil {
		for _, i := range fieldmappings.Items() {
			v_str, ok := i.Value.(string)
			if !ok {
				return nil, fmt.Errorf("fieldmapping for %s should be string, got(%T)",
					i.Key, i.Value)
			}

			// Compile it.
			lambda, err := vfilter.ParseLambda(v_str)
			if err != nil {
				return nil, fmt.Errorf("fieldmapping for %s is not a valid VQL Lambda: %v",
					i.Key, err)
			}
			self.fieldmappings.Set(i.Key, lambda)
		}
	}

	// Split rules into log sources
	for name, query := range log_sources.Queries() {
		runner := &SigmaExecutionContext{
			query: query,
			Name:  name,
		}
		log_target := parseLogSourceTarget(name)

		for _, r := range rules {
			if r.Logsource.Category == "" &&
				r.Logsource.Product == "" &&
				r.Logsource.Service == "" &&

				// Correlation rules are allowed to not have log sources
				r.Correlation == nil {
				scope.Log("INFO:sigma: Error parsing rule '%v': No logsource specified",
					r.Title)
				continue
			}

			// Filter out correlation rules.
			if r.Correlation == nil &&
				matchLogSource(log_target, r) {
				evaluator_rule := evaluator.NewVQLRuleEvaluator(
					scope, r, self.fieldmappings)

				// Check rule for sanity
				err := evaluator_rule.CheckRule()
				if err != nil {
					scope.Log("sigma: Error parsing: %v in rule '%v'",
						err, evaluator_rule.Rule.Title)
					continue
				}

				runner.rules = append(runner.rules, evaluator_rule)
				self.total_rules++

				if r.Name != "" {
					rules_by_name[r.Name] = evaluator_rule
				} else if r.ID != "" {
					rules_by_name[r.ID] = evaluator_rule
				}
			}
		}

		if len(runner.rules) > 0 {
			self.runners = append(self.runners, runner)
		}
	}

	// Prepare any correlations
	for _, r := range rules {
		if r.Correlation != nil {
			c, err := evaluator.NewSigmaCorrelator(scope, r, field_mapping_resolver)
			if err != nil {
				scope.Log("sigma: Correlation %v: Error in rule %v",
					r.Title, err)
				continue
			}

			for _, name := range r.Correlation.Rules {
				rule, pres := rules_by_name[name]
				if !pres {
					scope.Log("sigma: Correlation %v: References missing rule %v",
						r.Title, name)
					continue
				}

				rule.Correlator = c
			}
		}
	}

	self.pool = NewWorkerPool(ctx, &self.wg, self, output_chan)

	Tracker.Register(self)

	return self, nil
}

// A shallow copy of the dict
func toDict(scope vfilter.Scope, row vfilter.Row) *ordereddict.Dict {
	result, ok := row.(*ordereddict.Dict)
	if ok {
		return result
	}

	result = ordereddict.NewDict()
	for _, k := range scope.GetMembers(row) {
		v, _ := scope.Associative(row, k)
		result.Set(k, v)
	}
	return result
}
