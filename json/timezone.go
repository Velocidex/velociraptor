package json

import (
	"time"

	"github.com/Velocidex/json"
)

func GetJsonOptsForTimezone(timezone string) *json.EncOpts {
	if timezone == "" {
		return DefaultEncOpts()
	}

	loc := time.UTC
	if timezone != "" {
		loc, _ = time.LoadLocation(timezone)
	}

	return NewEncOpts().
		WithCallback(time.Time{},
			func(v interface{}, opts *json.EncOpts) ([]byte, error) {
				switch t := v.(type) {
				case time.Time:
					return t.In(loc).MarshalJSON()
				}
				return nil, json.EncoderCallbackSkip
			})
}
