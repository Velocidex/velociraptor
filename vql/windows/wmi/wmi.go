//go:build windows
// +build windows

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package wmi

import (
	"context"
	"errors"
	"io"
	"log"
	"runtime"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"

	wmi "github.com/microsoft/wmi/pkg/wmiinstance"
)

var (
	// ErrNilCreateObject is the error returned if CreateObject returns nil even
	// if the error was nil.
	ErrNilCreateObject = errors.New("wmi: create object returned nil")
	lock               sync.Mutex
)

func Query(query string, namespace string) (res []*ordereddict.Dict, err error) {
	// The github.com/microsoft/wmi can panic
	defer utils.CheckForPanic("WMI %s", query)

	lock.Lock()
	defer lock.Unlock()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if namespace == "" {
		namespace = "ROOT/CIMV2"
	}

	sm := wmi.NewWmiSessionManager()
	defer sm.Close()

	session, err := sm.GetLocalSession(namespace)
	if err != nil {
		return nil, err
	}
	_, err = session.Connect()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	rows, err := session.QueryInstances(query)
	if err != nil {
		return nil, err
	}

	// Iterating over the results
	for _, instance := range rows {
		row := ordereddict.NewDict().SetCaseInsensitive()
		properties := instance.GetClass().GetPropertiesNames()
		for _, property := range properties {
			fieldValue, err := instance.GetProperty(property)
			if err != nil {
				continue
			}

			row.Set(property, fieldValue)
		}
		res = append(res, row)
	}

	return res, err
}

// The VQL WMI plugin.
type WMIQueryArgs struct {
	Query     string `vfilter:"required,field=query,doc=The WMI query to issue."`
	Namespace string `vfilter:"optional,field=namespace,doc=The WMI namespace to use (ROOT/CIMV2)"`
}

func runWMIQuery(
	ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("wmi: %v", err)
		return result
	}

	arg := &WMIQueryArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("wmi: %v", err)
		return result
	}

	query_result, err := Query(arg.Query, arg.Namespace)
	if err != nil {
		scope.Log("wmi: %v", err)
		return result
	}
	for _, item := range query_result {
		result = append(result, item)
	}

	return result
}

func init() {
	log.SetOutput(io.Discard)
	vql_subsystem.RegisterPlugin(&vfilter.GenericListPlugin{
		PluginName: "wmi",
		Doc:        "Execute simple WMI queries synchronously.",
		Function:   runWMIQuery,
		ArgType:    &WMIQueryArgs{},
		Metadata:   vql_subsystem.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	})
}
