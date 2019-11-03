/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
package vql

import (
	"encoding/json"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	vfilter "www.velocidex.com/golang/vfilter"
)

func ExtractRows(vql_response *actions_proto.VQLResponse) ([]vfilter.Row, error) {
	result := []vfilter.Row{}
	var rows []map[string]interface{}
	err := json.Unmarshal([]byte(vql_response.Response), &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for _, row := range rows {
		item := ordereddict.NewDict()
		for k, v := range row {
			item.Set(k, v)
		}
		result = append(result, item)
	}

	return result, nil
}

func RowToDict(scope *vfilter.Scope, row vfilter.Row) *ordereddict.Dict {
	// If the row is already a dict nothing to do:
	result, ok := row.(*ordereddict.Dict)
	if ok {
		return result
	}

	result = ordereddict.NewDict()
	for _, column := range scope.GetMembers(row) {
		value, pres := scope.Associative(row, column)
		if pres {
			result.Set(column, value)
		}
	}

	return result
}

// GetStringFromRow gets a string value from row. If it is not there
// or not a string return ""
func GetStringFromRow(scope *vfilter.Scope,
	row vfilter.Row, key string) string {
	value, pres := scope.Associative(row, key)
	if pres {
		value_str, ok := value.(string)
		if ok {
			return value_str
		}
	}
	return ""
}

// GetIntFromRow gets a uint64 value from row. If it is not there
// or not a string return 0. Floats etc are coerced to uint64.
func GetIntFromRow(scope *vfilter.Scope,
	row vfilter.Row, key string) uint64 {
	value, pres := scope.Associative(row, key)
	if pres {
		switch t := value.(type) {
		case int:
			return uint64(t)
		case int8:
			return uint64(t)
		case int16:
			return uint64(t)
		case int32:
			return uint64(t)
		case uint8:
			return uint64(t)
		case uint16:
			return uint64(t)
		case uint32:
			return uint64(t)
		case uint64:
			return t
		}
	}
	return 0
}

// A writer which periodically reports how much has been
// written. Useful for tee with another writer.
type LogWriter struct {
	Scope   *vfilter.Scope
	Message string
	Period  time.Duration

	next_log   time.Time
	total_size int
}

func (self *LogWriter) Write(buff []byte) (int, error) {
	if self.Period == 0 {
		self.Period = 5 * time.Second
	}

	self.total_size += len(buff)
	if time.Now().After(self.next_log) {
		self.next_log = time.Now().Add(self.Period)
		self.Scope.Log("%s: Uploaded %v bytes",
			self.Message, self.total_size)
	}
	return len(buff), nil
}
