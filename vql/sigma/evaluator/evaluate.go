package evaluator

import (
	"context"
	"fmt"

	"github.com/bradleyjkemp/sigma-go"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type Result struct {
	Match            bool            // whether this event matches the Sigma rule
	SearchResults    map[string]bool // For each Search, whether it matched the event
	ConditionResults []bool          // For each Condition, whether it matched the event
}

type VQLRuleEvaluator struct {
	sigma.Rule

	fieldmappings map[string]*vfilter.Lambda
}

func NewVQLRuleEvaluator(rule sigma.Rule,
	fieldmappings map[string]*vfilter.Lambda) *VQLRuleEvaluator {
	// Make a local copy of the map so we dont need to lock it.

	result := &VQLRuleEvaluator{
		Rule:          rule,
		fieldmappings: make(map[string]*vfilter.Lambda),
	}

	for k, v := range fieldmappings {
		result.fieldmappings[k] = v
	}

	return result
}

// TODO: Not supported yet
func (self *VQLRuleEvaluator) evaluateAggregationExpression(
	ctx context.Context, conditionIndex int,
	aggregation sigma.AggregationExpr, event types.Row) (bool, error) {
	return false, nil
}

func (self *VQLRuleEvaluator) Match(ctx context.Context,
	scope types.Scope, event types.Row) (Result, error) {
	result := Result{
		Match:            false,
		SearchResults:    map[string]bool{},
		ConditionResults: make([]bool, len(self.Detection.Conditions)),
	}

	for identifier, search := range self.Detection.Searches {
		var err error
		result.SearchResults[identifier], err = self.evaluateSearch(ctx, scope, search, event)
		if err != nil {
			return Result{}, fmt.Errorf("error evaluating search %s: %w", identifier, err)
		}
	}

	for conditionIndex, condition := range self.Detection.Conditions {
		searchMatches := self.evaluateSearchExpression(condition.Search, result.SearchResults)

		switch {
		// Event didn't match filters
		case !searchMatches:
			result.ConditionResults[conditionIndex] = false
			continue

		// Simple query without any aggregation
		case searchMatches && condition.Aggregation == nil:
			result.ConditionResults[conditionIndex] = true
			result.Match = true
			continue // need to continue in case other conditions contain aggregations that need to be evaluated

		// Search expression matched but still need to see if the aggregation returns true
		case searchMatches && condition.Aggregation != nil:
			aggregationMatches, err := self.evaluateAggregationExpression(ctx, conditionIndex, condition.Aggregation, event)
			if err != nil {
				return Result{}, err
			}
			if aggregationMatches {
				result.Match = true
				result.ConditionResults[conditionIndex] = true
			}
			continue
		}
	}

	return result, nil
}
