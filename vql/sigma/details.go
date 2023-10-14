package sigma

import (
	"context"
	"regexp"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	expandRegEx = regexp.MustCompile("%[A-Za-z]+%")
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

			res, ok := resolved[0].(string)
			if ok {
				return res
			}
			return in
		})
	}

	return row.Copy().Set("Details", details)
}
