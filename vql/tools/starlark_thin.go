//go:build XXXXX
// +build XXXXX

package tools

import "www.velocidex.com/golang/vfilter/types"

type StarlModule struct{}

func (self *StarlModule) Marshal(scope types.Scope) (*types.MarshalItem, error) {
	return &types.MarshalItem{
		Type: "StarlModule",
	}, nil
}

func (self StarlModule) Unmarshal(unmarshaller types.Unmarshaller,
	scope types.Scope, item *types.MarshalItem) (interface{}, error) {
	return &StarlModule{}, nil
}
