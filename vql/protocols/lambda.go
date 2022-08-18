// Registers VQL Lambda functions as parameters

package protocols

import (
	"context"
	"errors"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	invalidLambdaError = errors.New("VQL lambda functions need to be a string of the form \"x=>...\"")
)

// Currently a lambda function needs to be a string.
func parseLambda(ctx context.Context, scope types.Scope, args *ordereddict.Dict,
	value interface{}) (interface{}, error) {

	switch t := value.(type) {
	case string:
		// Compile the batch lambda.
		return vfilter.ParseLambda(t)
	default:
		return nil, invalidLambdaError
	}
}

func init() {
	arg_parser.RegisterParser(&vfilter.Lambda{}, parseLambda)
}
