package vql

import (
	"github.com/shirou/gopsutil/process"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// Block potentially dangerous methods.
var _BlockedMembers = []string{"Terminate", "Kill", "Suspend", "Resume"}

type _ProcessFieldImpl struct{}

func (self _ProcessFieldImpl) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, b_ok := b.(string)
	switch a.(type) {
	case process.Process, *process.Process:
		return b_ok
	}
	return false
}

func (self _ProcessFieldImpl) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	field := b.(string)

	if utils.InString(&_BlockedMembers, field) {
		return false, true
	}

	res, pres := vfilter.DefaultAssociative{}.Associative(scope, a, b)
	return res, pres
}

func (self _ProcessFieldImpl) GetMembers(scope *vfilter.Scope, a vfilter.Any) []string {
	var result []string
	for _, item := range (vfilter.DefaultAssociative{}).GetMembers(scope, a) {
		if !utils.InString(&_BlockedMembers, item) {
			result = append(result, item)
		}
	}

	return result
}

func init() {
	exportedProtocolImpl = append(exportedProtocolImpl, &_ProcessFieldImpl{})
	exportedPlugins = append(exportedPlugins,
		vfilter.GenericListPlugin{
			PluginName: "pslist",
			Function: func(
				scope *vfilter.Scope,
				args *vfilter.Dict) []vfilter.Row {
				var result []vfilter.Row
				processes, err := process.Processes()
				if err == nil {
					for _, item := range processes {
						result = append(result, item)
					}
				}
				return result
			},
			RowType: process.Process{},
		})
}
