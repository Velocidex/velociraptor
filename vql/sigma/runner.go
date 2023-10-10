package sigma

import (
	"context"
	"fmt"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/bradleyjkemp/sigma-go"
	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type SigmaExecutionContext struct {
	query types.StoredQuery
	rules []*evaluator.VQLRuleEvaluator
}

type SigmaContext struct {
	runners []*SigmaExecutionContext

	// Map between sigma field names to event. The lambda will be
	// passed the event. For example EID can be the lambda
	// x=>x.System.EventID.Value
	fieldmappings map[string]*vfilter.Lambda

	debug       bool
	total_rules int
}

func (self *SigmaContext) SetDebug() {
	self.debug = true
}

func (self *SigmaContext) Rows(
	ctx context.Context, scope types.Scope) chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	var wg sync.WaitGroup

	for _, runner := range self.runners {
		wg.Add(1)
		go func() {
			defer wg.Done()

			subscope := scope.Copy()
			defer subscope.Close()

			for row := range runner.query.Eval(ctx, subscope) {
				// Evalute the row with all relevant rules
				for _, rule := range runner.rules {
					match, err := rule.Match(ctx, scope, row)
					if err != nil {
						continue
					}

					if !self.debug && !match.Match {
						continue
					}

					row_dict := vfilter.RowToDict(ctx, scope, row)
					row_dict.Set("_Match", match).
						Set("_Rule", rule.Title).
						Set("_References", rule.References).
						Set("Level", rule.Level)

					select {
					case <-ctx.Done():
						return
					case output_chan <- row_dict:
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(output_chan)

	}()

	return output_chan
}

func NewSigmaContext(
	scope types.Scope,
	rules []sigma.Rule,
	fieldmappings *ordereddict.Dict,
	log_sources *LogSourceProvider) (*SigmaContext, error) {

	// Compile the field mappings
	compiled_fieldmappings := make(map[string]*vfilter.Lambda)
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
			compiled_fieldmappings[k] = lambda
		}
	}

	var runners []*SigmaExecutionContext
	total_rules := 0

	// Split rules into log sources
	for name, query := range log_sources.queries {
		runner := &SigmaExecutionContext{
			query: query,
		}
		log_target := parseLogSourceTarget(name)

		for _, r := range rules {
			if matchLogSource(log_target, r) {
				runner.rules = append(runner.rules,
					evaluator.NewVQLRuleEvaluator(scope, r, compiled_fieldmappings))
				total_rules++
			}
		}

		if len(runner.rules) > 0 {
			runners = append(runners, runner)
		}
	}

	result := &SigmaContext{
		runners:       runners,
		fieldmappings: compiled_fieldmappings,
		total_rules:   total_rules,
	}
	return result, nil
}
