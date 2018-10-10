// VQL code for windows.
/* This file is adapted from github.com/StackExchange/wmi/wmi.go

We could not use that package directly because it only supports
extracting the OLE data to a Go struct but we really need it in a
vfilter.Dict().
*/

package wmi

import (
	"errors"
	"runtime"
	"sync"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	// ErrNilCreateObject is the error returned if CreateObject returns nil even
	// if the error was nil.
	ErrNilCreateObject = errors.New("wmi: create object returned nil")
	lock               sync.Mutex
)

// S_FALSE is returned by CoInitializeEx if it was already called on this thread.
const S_FALSE = 0x00000001

func Query(query string, namespace string) ([]*vfilter.Dict, error) {
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

	result := []*vfilter.Dict{}
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

			row := vfilter.NewDict().SetCaseInsensitive()
			for _, property := range properties {
				property_raw, err := item.GetProperty(property)
				if err != nil {
					row.Set(property, &vfilter.Null{})
					continue
				}

				row.Set(property, property_raw.Value())
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
	defer properties_raw.Clear()

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
	Query     string `vfilter:"required,field=query"`
	Namespace string `vfilter:"required,field=namespace"`
}

func runWMIQuery(scope *vfilter.Scope,
	args *vfilter.Dict) []vfilter.Row {
	var result []vfilter.Row
	arg := &WMIQueryArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("wmi: %s", err.Error())
		return result
	}

	query_result, err := Query(arg.Query, arg.Namespace)
	if err != nil {
		scope.Log("wmi: %s", err.Error())
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
	})
}
