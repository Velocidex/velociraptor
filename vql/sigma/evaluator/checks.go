package evaluator

import (
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/sigma-go"
	"www.velocidex.com/golang/vfilter"
)

func (self *VQLRuleEvaluator) CheckRule() error {
	// Rule has no condition - just and all the selections
	if self.Correlation == nil &&
		len(self.Detection.Conditions) == 0 {
		fields := []string{}
		for k := range self.Detection.Searches {
			fields = append(fields, k)
		}

		self.Detection.Conditions = append(self.Detection.Conditions,
			sigma.Condition{
				Search: sigma.SearchIdentifier{
					Name: strings.Join(fields, " and "),
				},
			})
	}

	if self.Detection.Timeframe != "" {
		return fmt.Errorf("In rule %v: Timeframe detections not supported",
			self.Title)
	}

	// Make sure if the rule has a VQL lambda it is valid.
	if self.AdditionalFields != nil {
		lambda_any, pres := self.AdditionalFields["vql"]
		if pres {
			lambda_str, ok := lambda_any.(string)
			if ok {
				lambda, err := vfilter.ParseLambda(lambda_str)
				if err != nil {
					return fmt.Errorf(
						"Rule provides invalid lambda: %v, Error: %v",
						lambda_str, err)
				}
				self.lambda = lambda
			}
		}

		enrichment_any, pres := self.AdditionalFields["enrichment"]
		if pres {
			enrichment_str, ok := enrichment_any.(string)
			if ok {
				enrichment, err := vfilter.ParseLambda(enrichment_str)
				if err != nil {
					return fmt.Errorf(
						"Rule provides invalid enrichment: %v, Error: %v",
						enrichment_str, err)
				}
				self.enrichment = enrichment
			}
		}

		self.lambda_args = ordereddict.NewDict()
		lambda_args_any, pres := self.AdditionalFields["vql_args"]
		if pres {
			lambda_args_dict, ok := lambda_args_any.(map[string]interface{})
			if ok {
				for k, v := range lambda_args_dict {
					self.lambda_args.Set(k, v)
				}
			} else {
				return fmt.Errorf("Rule %v: vql_args should be a dict",
					self.Title)
			}
		}
	}

	return nil
}
