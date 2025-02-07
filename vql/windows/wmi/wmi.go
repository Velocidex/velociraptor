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
// VQL code for windows.
/* This file is adapted from github.com/StackExchange/wmi/wmi.go

We could not use that package directly because it only supports
extracting the OLE data to a Go struct but we really need it in a
ordereddict.Dict().
*/

package wmi

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"github.com/Velocidex/ordereddict"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	// ErrNilCreateObject is the error returned if CreateObject returns nil even
	// if the error was nil.
	ErrNilCreateObject = errors.New("wmi: create object returned nil")
	lock               sync.Mutex
)

// S_FALSE is returned by CoInitializeEx if it was already called on this thread.
const S_FALSE = 0x00000001

func Query(query string, namespace string) ([]*ordereddict.Dict, error) {
	lock.Lock()
	defer lock.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if namespace == "" {
		namespace = "ROOT/CIMV2"
	}

	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	if err != nil {
		oleCode := err.(*ole.OleError).Code()
		if oleCode != ole.S_OK && oleCode != S_FALSE {
			return nil, err
		}
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, err
	} else if unknown == nil {
		return nil, ErrNilCreateObject
	}
	defer unknown.Release()

	wmi, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	defer wmi.Release()

	serviceRaw, err := oleutil.CallMethod(wmi, "ConnectServer", nil, namespace)
	if err != nil {
		return nil, err
	}

	service := serviceRaw.ToIDispatch()
	defer service.Release()

	resultRaw, err := oleutil.CallMethod(service, "ExecQuery", query)
	if err != nil {
		return nil, err
	}
	wmi_result := resultRaw.ToIDispatch()
	defer wmi_result.Release()

	result := []*ordereddict.Dict{}
	properties := []string{}

	err = oleutil.ForEach(wmi_result,
		func(v *ole.VARIANT) error {
			item := v.ToIDispatch()
			defer item.Release()

			if len(properties) == 0 {
				item_properties, err := getProperties(item)
				if err != nil {
					return err
				}
				properties = item_properties
			}

			row := ordereddict.NewDict().SetCaseInsensitive()
			for _, property := range properties {
				property_raw, err := item.GetProperty(property)
				if err != nil {
					row.Set(property, &vfilter.Null{})
					continue
				}

				defer func() {
					_ = property_raw.Clear()
				}()

				// If it is an array we convert it here.
				if property_raw.VT&ole.VT_ARRAY > 0 {
					result := []interface{}{}
					for _, item := range property_raw.ToArray().ToValueArray() {
						switch item.(type) {
						case *ole.IDispatch:
						case *ole.IUnknown:
						default:
							result = append(result, item)
						}
					}

					row.Set(property, result)
					continue
				}

				switch property_raw.VT {
				case ole.VT_UNKNOWN, ole.VT_DISPATCH:
					// Do not set these because we
					// cant do anything with them.

				default:
					row.Set(property, property_raw.Value())
				}
			}

			result = append(result, row)
			return nil
		})
	return result, err
}

func getProperties(item *ole.IDispatch) ([]string, error) {
	result := []string{}
	properties_raw, err := item.GetProperty("Properties_")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = properties_raw.Clear()
	}()

	properties := properties_raw.ToIDispatch()
	defer properties.Release()

	err = oleutil.ForEach(properties,
		func(v *ole.VARIANT) error {
			v_dispatch := v.ToIDispatch()
			defer v_dispatch.Release()

			name, err := v_dispatch.GetProperty("Name")
			if err != nil {
				return err
			}
			defer func() {
				_ = name.Clear()
			}()

			value, ok := name.Value().(string)
			if ok {
				result = append(result, value)
			}

			return nil
		})
	return result, err
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
	vql_subsystem.RegisterPlugin(&vfilter.GenericListPlugin{
		PluginName: "wmi",
		Doc:        "Execute simple WMI queries synchronously.",
		Function:   runWMIQuery,
		ArgType:    &WMIQueryArgs{},
		Metadata:   vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	})
}
