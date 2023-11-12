package protocols

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

func parseTime(ctx context.Context, scope types.Scope, args *ordereddict.Dict,
	value interface{}) (interface{}, error) {
	result, err := functions.TimeFromAny(ctx, scope, value)
	if err != nil {
		return time.Time{}, nil
	}

	return result, nil
}

func init() {
	arg_parser.RegisterParser(time.Time{}, parseTime)
}
