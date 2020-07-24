package vql

import (
	"bytes"
	"time"

	"github.com/Velocidex/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func EncOptsFromScope(scope *vfilter.Scope) *json.EncOpts {
	location := time.UTC

	location_name, pres := scope.Resolve("TZ")
	if pres {
		location_str, ok := location_name.(string)
		if ok {
			l, err := time.LoadLocation(location_str)
			if err == nil {
				location = l
			}
		}
	}

	cb := func(v interface{}, opts *json.EncOpts) ([]byte, error) {
		switch t := v.(type) {
		case time.Time:
			// Marshal the time in the desired timezone.
			return t.In(location).MarshalJSON()

		case *time.Time:
			return t.In(location).MarshalJSON()

		case utils.TimeVal:
			return t.Time().In(location).MarshalJSON()

		case *utils.TimeVal:
			return t.Time().In(location).MarshalJSON()

		}
		return nil, json.EncoderCallbackSkip
	}

	// Override time handling to support scope timezones
	return vjson.NewEncOpts().
		WithCallback(time.Time{}, cb).
		WithCallback(&time.Time{}, cb).
		WithCallback(utils.TimeVal{}, cb).
		WithCallback(&utils.TimeVal{}, cb)
}

// Utilities for encoding json via the vfilter API.
func MarshalJson(scope *vfilter.Scope) vfilter.RowEncoder {
	return func(rows []vfilter.Row) ([]byte, error) {
		return json.MarshalWithOptions(rows, EncOptsFromScope(scope))
	}
}

func MarshalJsonIndent(scope *vfilter.Scope) vfilter.RowEncoder {
	return func(rows []vfilter.Row) ([]byte, error) {
		b, err := json.MarshalWithOptions(rows, EncOptsFromScope(scope))
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		err = json.Indent(&buf, b, "", " ")
		if err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
}

func MarshalJsonl(scope *vfilter.Scope) vfilter.RowEncoder {
	return func(rows []vfilter.Row) ([]byte, error) {
		options := EncOptsFromScope(scope)
		out := bytes.Buffer{}
		for _, row := range rows {
			serialized, err := json.MarshalWithOptions(
				row, options)
			if err != nil {
				return nil, err
			}
			out.Write(serialized)
		}
		return out.Bytes(), nil
	}
}
