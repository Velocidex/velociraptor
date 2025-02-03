package evaluator

import (
	"context"
	"path"
	"reflect"
	"strings"

	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator/modifiers"
	"www.velocidex.com/golang/vfilter/types"

	"github.com/Velocidex/sigma-go"
)

func (self *VQLRuleEvaluator) evaluateSearchExpression(
	search sigma.SearchExpr, searchResults map[string]bool) bool {
	switch s := search.(type) {
	case sigma.And:
		for _, node := range s {
			if !self.evaluateSearchExpression(node, searchResults) {
				return false
			}
		}
		return true

	case sigma.Or:
		for _, node := range s {
			if self.evaluateSearchExpression(node, searchResults) {
				return true
			}
		}
		return false

	case sigma.Not:
		return !self.evaluateSearchExpression(s.Expr, searchResults)

	case sigma.SearchIdentifier:
		// If `s.Name` is not defined, this is always false
		return searchResults[s.Name]

	case sigma.OneOfThem:
		for name := range self.Detection.Searches {
			if self.evaluateSearchExpression(sigma.SearchIdentifier{Name: name}, searchResults) {
				return true
			}
		}
		return false

	case sigma.OneOfPattern:
		for name := range self.Detection.Searches {
			// it's not possible for this call to error because the
			// search expression parser won't allow this to contain
			// invalid expressions
			matchesPattern, _ := path.Match(s.Pattern, name)
			if !matchesPattern {
				continue
			}
			if self.evaluateSearchExpression(sigma.SearchIdentifier{Name: name}, searchResults) {
				return true
			}
		}
		return false

	case sigma.AllOfThem:
		for name := range self.Detection.Searches {
			if !self.evaluateSearchExpression(sigma.SearchIdentifier{Name: name}, searchResults) {
				return false
			}
		}
		return true

	case sigma.AllOfPattern:
		for name := range self.Detection.Searches {
			// it's not possible for this call to error because the
			// search expression parser won't allow this to contain
			// invalid expressions
			matchesPattern, _ := path.Match(s.Pattern, name)
			if !matchesPattern {
				continue
			}
			if !self.evaluateSearchExpression(sigma.SearchIdentifier{Name: name}, searchResults) {
				return false
			}
		}
		return true
	}
	self.scope.Log("ERROR:unhandled node type %T", search)
	return false
}

func (self *VQLRuleEvaluator) evaluateSearch(
	ctx context.Context, scope types.Scope,
	search sigma.Search, event *Event) (bool, error) {
	if len(search.Keywords) > 0 {
		event_str := event.AsJson()

		// A keyword match occurs over the entire event.
		for _, kw := range search.Keywords {
			if strings.Contains(event_str, strings.ToLower(kw)) {
				return true, nil
			}
		}

		return false, nil
	}

	if len(search.EventMatchers) == 0 {
		// degenerate case (but common for logsource conditions)
		return true, nil
	}

	// A Search is a series of EventMatchers (usually one).
	// Each EventMatchers is a series of "does this field match this value" conditions
	// all fields need to match for an EventMatcher to match, but only one EventMatcher needs to match for the Search to evaluate to true
eventMatcher:
	for _, eventMatcher := range search.EventMatchers {
		for _, fieldMatcher := range eventMatcher {

			// Get the field value.
			values, err := self.GetFieldValuesFromEvent(
				ctx, scope, fieldMatcher.Field, event)
			if err != nil {
				return false, err
			}

			// Get all relevant modifiers
			modifiers, err := modifiers.GetModifiers(fieldMatcher.Modifiers)
			if err != nil {
				return false, err
			}

			// Match using these modifiers
			if !self.applyModifiers(
				ctx, scope,
				fieldMatcher.Values, modifiers, values) {

				// this field didn't match so the overall matcher
				// doesn't match, try the next EventMatcher
				continue eventMatcher
			}
		}

		// all fields matched!
		return true, nil
	}

	// None of the event matchers explicitly matched
	return false, nil
}

func (self *VQLRuleEvaluator) applyModifiers(
	ctx context.Context, scope types.Scope,
	expected []interface{},
	mods []modifiers.ValueModifier,
	values []interface{}) bool {

	for _, m := range mods {
		new_values, new_expected, err := m.Modify(ctx, scope, values, expected)
		if err != nil {
			scope.Log("Sigma: %v\n", err)
			return false
		}
		values = new_values
		expected = new_expected
	}

	// If any value is true return true
	for _, v := range values {
		if scope.Bool(v) {
			return true
		}
	}
	return false
}

func (self *VQLRuleEvaluator) GetFieldValuesFromEvent(
	ctx context.Context, scope types.Scope,
	field string, event *Event) ([]interface{}, error) {

	// There is a field mapping - lets evaluate it
	lambda, err := self.fieldmappings.Get(field)
	if err == nil {
		return toGenericSlice(event.Reduce(ctx, scope, field, lambda)), nil
	}

	value, ok := event.Get(field)
	if !ok {
		return nil, nil
	}

	return toGenericSlice(value), nil
}

func toGenericSlice(v interface{}) []interface{} {
	rv := reflect.ValueOf(v)

	// if this isn't a slice, then return a slice containing the
	// original value
	if rv.Kind() != reflect.Slice {
		return []interface{}{v}
	}

	var out []interface{}
	for i := 0; i < rv.Len(); i++ {
		out = append(out, rv.Index(i).Interface())
	}

	return out
}
