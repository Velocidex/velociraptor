//go:build windows && amd64 && cgo
// +build windows,amd64,cgo

package process

import (
	"github.com/Velocidex/etw"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	kernel_info_managerTag = "$KIM"
)

func GetKernelInfoManager(scope vfilter.Scope) *etw.KernelInfoManager {
	kim_any := vql_subsystem.CacheGet(scope, kernel_info_managerTag)
	if utils.IsNil(kim_any) {
		kim_any = etw.NewKernelInfoManager()
		vql_subsystem.CacheSet(scope, kernel_info_managerTag, kim_any)
	}

	kim, ok := kim_any.(*etw.KernelInfoManager)
	if !ok {
		kim = etw.NewKernelInfoManager()
		vql_subsystem.CacheSet(scope, kernel_info_managerTag, kim)
	}

	return kim
}
