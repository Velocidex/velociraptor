package sigma

import (
	"strings"

	"github.com/bradleyjkemp/sigma-go"
)

func CheckRule(rule *sigma.Rule) error {
	// Rule has no condition - just and all the selections
	if len(rule.Detection.Conditions) == 0 {
		fields := []string{}
		for k := range rule.Detection.Searches {
			fields = append(fields, k)
		}

		rule.Detection.Conditions = append(rule.Detection.Conditions,
			sigma.Condition{
				Search: sigma.SearchIdentifier{
					Name: strings.Join(fields, " and "),
				},
			})
	}

	return nil
}
