package vql

import (
	"bytes"
	"time"

	"github.com/Velocidex/json"
	"www.velocidex.com/golang/velociraptor/constants"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

func EncOptsFromScope(scope vfilter.Scope) *json.EncOpts {
	// Default formatter should be very fast.
	cb := func(v interface{}, opts *json.EncOpts) ([]byte, error) {
		switch t := v.(type) {
		case time.Time:
			// Marshal the time in the UTC timezone by default.
			return t.UTC().MarshalJSON()

		case *time.Time:
			return t.UTC().MarshalJSON()

		}
		return nil, json.EncoderCallbackSkip
	}

	// If the scope contains a TZ variable, then we will use that
	// instead.
	location_name, pres := scope.Resolve(constants.TZ)
	if pres {
		location_str, ok := location_name.(string)
		if ok {
			// If we can not find the time zone just
			// ignore it.
			l, err := time.LoadLocation(location_str)
			if err == nil {
				cb = func(v interface{}, opts *json.EncOpts) ([]byte, error) {
					switch t := v.(type) {
					case time.Time:
						// Marshal the time in the desired timezone.
						return t.In(l).MarshalJSON()

					case *time.Time:
						return t.In(l).MarshalJSON()

					}
					return nil, json.EncoderCallbackSkip
				}
			}
		}
	}

	// Override time handling to support scope timezones
	return vjson.NewEncOpts().
		WithCallback(time.Time{}, cb).
		WithCallback(&time.Time{}, cb)
}

// Utilities for encoding json via the vfilter API.
func MarshalJson(scope vfilter.Scope) vfilter.RowEncoder {
	opts := EncOptsFromScope(scope)
	return func(rows []vfilter.Row) ([]byte, error) {
		return json.MarshalWithOptions(rows, opts)
	}
}

func MarshalJsonIndent(scope vfilter.Scope) vfilter.RowEncoder {
	opts := EncOptsFromScope(scope)
	return func(rows []vfilter.Row) ([]byte, error) {
		b, err := json.MarshalWithOptions(rows, opts)
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

func MarshalJsonIndentIgnoreEmpty(scope vfilter.Scope) vfilter.RowEncoder {
	opts := EncOptsFromScope(scope)
	return func(rows []vfilter.Row) ([]byte, error) {
		// Ignore empty rows
		if len(rows) == 0 {
			return nil, nil
		}

		b, err := json.MarshalWithOptions(rows, opts)
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

func MarshalJsonl(scope vfilter.Scope) vfilter.RowEncoder {
	options := EncOptsFromScope(scope)

	return func(rows []vfilter.Row) ([]byte, error) {
		out := bytes.Buffer{}
		for _, row := range rows {
			serialized, err := json.MarshalWithOptions(row, options)
			if err != nil {
				return nil, err
			}
			out.Write(serialized)
			out.Write([]byte("\n"))
		}
		return out.Bytes(), nil
	}
}
