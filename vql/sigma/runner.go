package sigma

import (
	"context"
	"fmt"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/bradleyjkemp/sigma-go"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type SigmaExecutionContext struct {
	Name string

	query types.StoredQuery
	rules []*evaluator.VQLRuleEvaluator
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

				// Evalute the row with all relevant rules
				self.pool.Run(ctx, subscope,
					evaluator.NewEvent(row_dict), runner.rules)
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

	// Compile the field mappings.  NOTE: The compiled_fieldmappings
	// is shared between all the worker goroutines. Benchmarking shows
	// it is faster to make a slice copy than having to use a mutex to
	// protect it. This is O(1) but lock free. Using map copies uses
	// up significant amount of memory for local map copies.
	var compiled_fieldmappings []evaluator.FieldMappingRecord
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
			compiled_fieldmappings = append(compiled_fieldmappings,
				evaluator.FieldMappingRecord{Name: k, Lambda: lambda})
		}
	}

	var runners []*SigmaExecutionContext
	total_rules := 0

	// Split rules into log sources
	for name, query := range log_sources.queries {
		runner := &SigmaExecutionContext{
			query: query,
			Name:  name,
		}
		log_target := parseLogSourceTarget(name)

		for _, r := range rules {
			if matchLogSource(log_target, r) {
				evaluator_rule := evaluator.NewVQLRuleEvaluator(
					scope, r, compiled_fieldmappings)

				// Check rule for sanity
				err := evaluator_rule.CheckRule()
				if err != nil {
					scope.Log("sigma: Error parsing: %v in rule '%v'",
						err, evaluator_rule.Rule.Title)
					continue
				}

				runner.rules = append(runner.rules, evaluator_rule)
				total_rules++
			}
		}

		if len(runner.rules) > 0 {
			runners = append(runners, runner)
		}
	}

	output_chan := make(chan vfilter.Row)
	result := &SigmaContext{
		output_chan:     output_chan,
		runners:         runners,
		fieldmappings:   compiled_fieldmappings,
		total_rules:     total_rules,
		default_details: default_details,
		debug:           debug,
	}
	result.pool = NewWorkerPool(ctx, &result.wg, result, output_chan)
	return result, nil
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
