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
	expandRegEx = regexp.MustCompile("%[A-Z.a-z_0-9]+%")
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
			in = in[1 : len(in)-1]

			resolved, err := rule.GetFieldValuesFromEvent(ctx, scope, in, row)
			if err != nil || len(resolved) == 0 {
				return in
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
