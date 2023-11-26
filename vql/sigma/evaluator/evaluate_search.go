package evaluator

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"strings"

	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator/modifiers"
	"www.velocidex.com/golang/vfilter/types"

	"github.com/bradleyjkemp/sigma-go"
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

	// A Search is a series of EventMatchers (usually one)
	// Each EventMatchers is a series of "does this field match this value" conditions
	// all fields need to match for an EventMatcher to match, but only one EventMatcher needs to match for the Search to evaluate to true
eventMatcher:
	for _, eventMatcher := range search.EventMatchers {
		for _, fieldMatcher := range eventMatcher {
			// A field matcher can specify multiple values to match against
			// either the field should match all of these values or it should match any of them
			allValuesMustMatch := false
			fieldModifiers := fieldMatcher.Modifiers
			if len(fieldMatcher.Modifiers) > 0 && fieldModifiers[len(fieldModifiers)-1] == "all" {
				allValuesMustMatch = true
				fieldModifiers = fieldModifiers[:len(fieldModifiers)-1]
			}

			// field matchers can specify modifiers (FieldName|modifier1|modifier2) which change the matching behaviour
			comparator, err := modifiers.GetComparator(fieldModifiers...)
			if err != nil {
				return false, err
			}

			matcherValues, err := self.getMatcherValues(ctx, fieldMatcher)
			if err != nil {
				return false, err
			}

			values, err := self.GetFieldValuesFromEvent(
				ctx, scope, fieldMatcher.Field, event)
			if err != nil {
				return false, err
			}
			if !self.matcherMatchesValues(matcherValues, comparator, allValuesMustMatch, values) {
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

func (self *VQLRuleEvaluator) getMatcherValues(ctx context.Context, matcher sigma.FieldMatcher) ([]string, error) {
	matcherValues := []string{}
	for _, abstractValue := range matcher.Values {
		value := ""

		switch abstractValue := abstractValue.(type) {
		case string:
			value = abstractValue
		case int, float32, float64, bool:
			value = fmt.Sprintf("%v", abstractValue)
		default:
			return nil, fmt.Errorf("expected scalar field matching value got: %v (%T)", abstractValue, abstractValue)
		}

		matcherValues = append(matcherValues, value)
	}
	return matcherValues, nil
}

func (self *VQLRuleEvaluator) GetFieldValuesFromEvent(
	ctx context.Context, scope types.Scope,
	field string, event *Event) ([]interface{}, error) {

	// There is a field mapping - lets evaluate it
	for _, m := range self.fieldmappings {
		if m.Name == field {
			return toGenericSlice(event.Reduce(ctx, scope, field, m.Lambda)), nil
		}
	}

	value, ok := event.Get(field)
	if !ok {
		return nil, nil
	}

	return toGenericSlice(value), nil
}

func (self *VQLRuleEvaluator) matcherMatchesValues(
	matcherValues []string, comparator modifiers.ComparatorFunc, allValuesMustMatch bool, actualValues []interface{}) bool {
	matched := allValuesMustMatch
	for _, expectedValue := range matcherValues {
		valueMatchedEvent := false
		// There are multiple possible event fields that each expected
		// value needs to be compared against
		for _, actualValue := range actualValues {
			comparatorMatched, err := comparator(actualValue, expectedValue)
			if err != nil {
				// todo
			}
			if comparatorMatched {
				valueMatchedEvent = true
				break
			}
		}

		if allValuesMustMatch {
			matched = matched && valueMatchedEvent
		} else {
			matched = matched || valueMatchedEvent
		}
	}
	return matched
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
