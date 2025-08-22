package sigma

import (
	"context"
	"regexp"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	expandRegEx = regexp.MustCompile(`%([A-Z.a-z_0-9]+)(\[([0-9]+)\])?%`)
)

func (self *SigmaContext) AddDetail(
	ctx context.Context,
	scope vfilter.Scope,
	row *evaluator.Event,
	rule *evaluator.VQLRuleEvaluator) *ordereddict.Dict {

	details, pres := rule.AdditionalFields["details"]
	if !pres {
		// Try to get the default details using the details lambda.
		if self.default_details != nil {
			details = self.default_details.Reduce(ctx, scope, []types.Any{row.Dict})
		}

		if utils.IsNil(details) {
			details = &vfilter.Null{}
		}
	}

	details_str, ok := details.(string)
	if ok {
		details = expandRegEx.ReplaceAllStringFunc(details_str, func(in string) string {
			if len(in) <= 2 {
				return in
			}

			match := expandRegEx.FindStringSubmatch(in)
			variable := match[1]

			// Support indexed expressions like %Data[1]%
			index := int64(-1)
			if len(match) == 4 {
				parsed_index, ok := utils.ToInt64(match[3])
				if ok {
					// Indexes are 1 based - i.e. first element is %Data[1]%
					index = parsed_index - 1
				}
			}

			resolved, err := rule.GetFieldValuesFromEvent(ctx, scope, variable, row)
			if err != nil || len(resolved) == 0 {
				return in
			}

			if index != -1 {
				if index < int64(len(resolved)) {
					resolved = []interface{}{resolved[index]}
				} else {
					return ""
				}
			}

			if len(resolved) == 1 {
				res, ok := resolved[0].(string)
				if ok {
					return res
				}

				// If it is not a string, serialize to json and
				// interpolate instead.
				serialized, err := json.Marshal(resolved[0])
				if err == nil {
					return string(serialized)
				}
			}

			// Handle lists and dicts
			serialized, err := json.Marshal(resolved)
			if err == nil {
				return string(serialized)
			}

			return in
		})
	}

	return row.Set("Details", details)
}
