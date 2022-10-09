package grouper

import (
	"context"
	"io"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type TestGroupbyActor struct {
	idx       int
	rows      []*ordereddict.Dict
	gb_column string
}

func (self *TestGroupbyActor) GetNextRow(
	ctx context.Context, scope types.Scope) (
	types.LazyRow, string, types.Scope, error) {
	if self.idx < len(self.rows) {
		res := self.rows[self.idx]
		self.idx++
		value, _ := res.GetString(self.gb_column)
		lazy_row := vfilter.NewLazyRow(ctx, scope)
		for _, k := range res.Keys() {
			value, _ := res.Get(k)
			lazy_row.AddColumn(k,
				func(ctx context.Context, scope types.Scope) types.Any {
					return value
				})
		}
		return lazy_row, value, scope, nil
	}
	return nil, "", scope, io.EOF
}

func (self *TestGroupbyActor) MaterializeRow(ctx context.Context,
	row types.Row, scope types.Scope) *ordereddict.Dict {
	return vfilter.MaterializedLazyRow(ctx, row, scope)
}
