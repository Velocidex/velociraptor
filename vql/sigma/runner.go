package sigma

import (
	"context"
	"fmt"
	"sync"

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
	Name string

	query types.StoredQuery

	// Rules not using correlations - can be run in any order.
	rules []*evaluator.VQLRuleEvaluator

	// Rules consumed by correlations - must be run in order
	correlations     []*evaluator.VQLRuleEvaluator
	non_correlations []*evaluator.VQLRuleEvaluator
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

type SigmaContext struct {
	runners []*SigmaExecutionContext

	// Map between sigma field names to event. The lambda will be
	// passed the event. For example EID can be the lambda
	// x=>x.System.EventID.Value
	fieldmappings []evaluator.FieldMappingRecord

	mu          sync.Mutex
	debug       bool
	total_rules int
	hit_count   int

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

	for _, runner := range self.runners {
		subscope := scope.Copy()
		runner.balance()

		self.wg.Add(1)
		go func(runner *SigmaExecutionContext) {
			defer self.wg.Done()
			defer subscope.Close()

			count := 0
			start := utils.GetTime().Now()

			defer func() {
				scope.Log("INFO:sigma: Consumed %v messages from log source %v on %v rules (%v)",
					count, runner.Name, len(runner.rules),
					utils.GetTime().Now().Sub(start))
			}()

			query_log := actions.QueryLog.AddQuery(
				vfilter.FormatToString(subscope, runner.query))
			defer query_log.Close()

			for row := range runner.query.Eval(ctx, subscope) {
				count++

				row_dict := toDict(subscope, row)

				// Evalute the row with all relevant
				// rules. Correlations are evaluted inline because
				// they need to be ordered.
				if len(runner.correlations) > 0 {
					self.pool.RunInline(ctx, subscope,
						evaluator.NewEvent(row_dict), runner.correlations)
				}

				// Evaluate the rest of the rules in parallel.
				if len(runner.non_correlations) > 0 {
					self.pool.Run(ctx, subscope,
						evaluator.NewEvent(row_dict), runner.non_correlations)
				}
			}
		}(runner)
	}

	go func() {
		self.wg.Wait()
		close(self.output_chan)
	}()

	return self.output_chan
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

	self := &SigmaContext{
		output_chan:     output_chan,
		default_details: default_details,
		debug:           debug,
	}

	// Compile the field mappings.  NOTE: The compiled_fieldmappings
	// is shared between all the worker goroutines. Benchmarking shows
	// it is faster to make a slice copy than having to use a mutex to
	// protect it. This is O(1) but lock free. Using map copies uses
	// up significant amount of memory for local map copies.
	if fieldmappings != nil {
		for _, k := range fieldmappings.Keys() {
			v, _ := fieldmappings.Get(k)
			v_str, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("fieldmapping for %s should be string, got(%T)", k, v)
			}

			// Compile it.
			lambda, err := vfilter.ParseLambda(v_str)
			if err != nil {
				return nil, fmt.Errorf("fieldmapping for %s is not a valid VQL Lambda: %v", k, err)
			}
			self.fieldmappings = append(self.fieldmappings,
				evaluator.FieldMappingRecord{Name: k, Lambda: lambda})
		}
	}

	// Split rules into log sources
	for name, query := range log_sources.queries {
		runner := &SigmaExecutionContext{
			query: query,
			Name:  name,
		}
		log_target := parseLogSourceTarget(name)

		for _, r := range rules {
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
			c := evaluator.NewSigmaCorrelator(r)

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
