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
package parsers

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/reflect/protoreflect"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	json_tools "www.velocidex.com/golang/velociraptor/tools/json"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/parsers/syslog"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	BUFF_SIZE = 10 * 1024 * 1024
)

type ParseJsonFunctionArg struct {
	Data   string   `vfilter:"required,field=data,doc=Json encoded string."`
	Schema []string `vfilter:"optional,field=schema,doc=Json schema to use for validation."`
}
type ParseJsonFunction struct{}

func (self ParseJsonFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_json",
		Doc:     "Parse a JSON string into an object.",
		ArgType: type_map.AddType(scope, &ParseJsonFunctionArg{}),
		Version: 2,
	}
}

func (self ParseJsonFunction) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "parse_json", args)()

	arg := &ParseJsonFunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_json: %v", err)
		return &vfilter.Null{}
	}

	if len(arg.Schema) > 0 {
		var options json_tools.ValidationOptions
		result, errs := json_tools.ParseJsonToObjectWithSchema(
			arg.Data, arg.Schema, options)
		if len(errs) > 0 {
			for _, err := range errs {
				scope.Log("ERROR:parse_json: %v", err)
			}
			return &vfilter.Null{}
		}
		return result
	}

	result, err := utils.ParseJsonToObject([]byte(arg.Data))
	if err != nil {
		scope.Log("parse_json: %v: %v", err, utils.Elide(arg.Data, 100))
		return &vfilter.Null{}
	}
	return result
}

type ParseJsonArray struct{}

func (self ParseJsonArray) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_json_array",
		Doc:     "Parse a JSON string into an array.",
		ArgType: type_map.AddType(scope, &ParseJsonFunctionArg{}),
	}
}

func (self ParseJsonArray) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "parse_json_array", args)()

	arg := &ParseJsonFunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_json_array: %v", err)
		return &vfilter.Null{}
	}

	arg.Data = strings.TrimSpace(arg.Data)

	result_array := []json.RawMessage{}
	if arg.Data == "" {
		return result_array
	}

	err = json.Unmarshal([]byte(arg.Data), &result_array)
	if err != nil {
		scope.Log("parse_json_array: %v", err)
		return &vfilter.Null{}
	}

	result := make([]vfilter.Any, 0, len(result_array))
	for _, item := range result_array {
		dict, err := utils.ParseJsonToObject(item)
		if err != nil {
			// It might not be a dict - support any value.
			var any_value interface{}
			err = json.Unmarshal(item, &any_value)
			if err != nil {
				scope.Log("parse_json_array: %v", err)
				return &vfilter.Null{}
			}

			result = append(result, any_value)
			continue
		}

		result = append(result, dict)
	}

	return result
}

type ParseJsonlPluginArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=JSON file to open"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
}

type ParseJsonlPlugin struct{}

func (self ParseJsonlPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "parse_jsonl", args)()

		arg := &ParseJsonlPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_jsonl: %s", err.Error())
			return
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("parse_jsonl: %v", err)
			return
		}

		fd, err := accessor.OpenWithOSPath(arg.Filename)
		if err != nil {
			scope.Log("Unable to open file %s: %v",
				arg.Filename, err)
			return
		}
		defer fd.Close()

		count := 0
		reader := bufio.NewReader(fd)
		for {
			select {
			case <-ctx.Done():
				return

			default:
				row_data, err := reader.ReadBytes('\n')
				// Need to at least read something to make progress.
				if len(row_data) == 0 {
					return
				}

				// Report errors
				if err != nil && !errors.Is(err, io.EOF) {
					scope.Log("parse_jsonl: %v", err)
					return
				}

				// Skip empty lines
				if len(row_data) == 1 {
					continue
				}

				count++
				var item vfilter.Row

				// This looks like a dict - parse it as and ordered
				// dict so we preserve key order.
				if row_data[0] == '{' {
					item_dict, err := utils.ParseJsonToObject(row_data)
					if err != nil {
						// Skip lines that are not valid - they might be corrupted
						functions.DeduplicatedLog(ctx, scope,
							"parse_jsonl: error at line %v: %v skipping all invalid lines", count, err)
						continue
					}
					item = item_dict

					// Otherwise parse it as whatever it looks like
					// and return a row with _value column in it
				} else {
					err = json.Unmarshal(row_data, &item)
					if err != nil {
						// Skip lines that are not valid - they might be corrupted
						functions.DeduplicatedLog(ctx, scope,
							"parse_jsonl: error at line %v: %v skipping all invalid lines", count, err)
						continue
					}

					item = ordereddict.NewDict().Set("_value", item)
				}

				select {
				case <-ctx.Done():
					return

				case output_chan <- item:
				}
			}
		}
	}()

	return output_chan
}

func (self ParseJsonlPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_jsonl",
		Doc:      "Parses a line oriented json file.",
		ArgType:  type_map.AddType(scope, &ParseJsonlPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type ParseJsonArrayPlugin struct{}

func (self ParseJsonArrayPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "parse_json_array", args)()

		result := ParseJsonArray{}.Call(ctx, scope, args)
		result_value := reflect.Indirect(reflect.ValueOf(result))
		result_type := result_value.Type()
		if result_type.Kind() == reflect.Slice {
			for i := 0; i < result_value.Len(); i++ {
				select {
				case <-ctx.Done():
					return

				case output_chan <- result_value.Index(i).Interface():
				}
			}
		}

	}()

	return output_chan
}

func (self ParseJsonArrayPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_json_array",
		Doc:     "Parses events from a line oriented json file.",
		ArgType: type_map.AddType(scope, &ParseJsonFunctionArg{}),
	}
}

// Associative protocol for map[string]interface{}
type _MapInterfaceAssociativeProtocol struct{}

func (self _MapInterfaceAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {

	a_type := reflect.TypeOf(a)
	if a_type == nil {
		return false
	}
	if a_type.Kind() != reflect.Map {
		return false
	}

	_, b_ok := b.(string)
	return b_ok
}

func (self _MapInterfaceAssociativeProtocol) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	key, key_ok := b.(string)
	map_value := reflect.ValueOf(a)
	if key_ok && map_value.Kind() == reflect.Map {
		lower_key := strings.ToLower(key)
		for _, map_key_value := range map_value.MapKeys() {
			map_key := map_key_value.String()
			// Try a case insensitive match.
			if map_key == key ||
				strings.ToLower(map_key) == lower_key {
				result := map_value.MapIndex(map_key_value)
				if !utils.IsNil(result) {
					return result.Interface(), true
				}
			}
		}
	}
	return vfilter.Null{}, false
}

func (self _MapInterfaceAssociativeProtocol) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {

	result := []string{}
	map_value := reflect.ValueOf(a)
	if map_value.Kind() == reflect.Map {
		for _, map_key_value := range map_value.MapKeys() {
			result = append(result, map_key_value.String())

		}
	}

	return result
}

/*
When JSON encoding a protobuf, the output uses the original
protobuf field names, however within Go they are converted to go
style. For example if the protobuf has os_info, then Go fields will
be OsInfo.

This is very confusing to users since they first use SELECT * from
plugin(), the * expands to Associative.GetMembers(). This should emit
the field names that occur in the JSON. The user will then attempt to
select such a field, and Associative() should therefore convert to
the go style automatically.
*/
type _ProtobufAssociativeProtocol struct{}

func (self _ProtobufAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {

	_, b_ok := b.(string)
	if b_ok {
		switch a.(type) {
		case protoreflect.ProtoMessage:
			return true
		}
	}

	return false
}

// Accept either the json emitted field name or the go style field
// name.
func (self _ProtobufAssociativeProtocol) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	field, b_ok := b.(string)
	if !b_ok {
		return nil, false
	}

	if reflect.ValueOf(a).IsNil() {
		return nil, false
	}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()

	// Protobuf reflection API V2 is far too complicated - this is
	// a hack but works ok for now.
	for i := 0; i < a_type.NumField(); i++ {
		struct_field := a_type.Field(i)
		if field == struct_field.Name {
			field_value := a_value.Field(i)
			if field_value.CanInterface() {
				return field_value.Interface(), true
			}
		}

		json_tag := strings.Split(struct_field.Tag.Get("json"), ",")
		if field == json_tag[0] {
			field_value := a_value.Field(i)
			if field_value.CanInterface() {
				return a_value.Field(i).Interface(), true
			}
		}
	}
	return vfilter.Null{}, false
}

// Emit the json serializable field name only. This makes this field
// consistent with the same protobuf emitted as json using other
// means.
func (self _ProtobufAssociativeProtocol) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {
	result := []string{}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()

	for i := 0; i < a_type.NumField(); i++ {
		struct_field := a_type.Field(i)
		json_tag := strings.Split(struct_field.Tag.Get("json"), ",")[0]
		if json_tag != "" {
			result = append(result, json_tag)
		}
	}
	return result
}

type _nilAssociativeProtocol struct{}

func (self _nilAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {

	value := reflect.ValueOf(a)
	return value.Kind() == reflect.Ptr && value.IsNil()
}

func (self _nilAssociativeProtocol) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	return vfilter.Null{}, false
}

func (self _nilAssociativeProtocol) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {
	return []string{}
}

// Allow a slice to be accessed by a field
type _IndexAssociativeProtocol struct{}

func (self _IndexAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {
	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()
	if a_type.Kind() != reflect.Slice {
		return false
	}

	switch t := b.(type) {
	case string:
		_, err := strconv.Atoi(t)
		if err == nil {
			return true
		}
	case int, float64, uint64, int64, *int, *float64, *uint64, *int64:
		return true
	}
	return false
}

func (self _IndexAssociativeProtocol) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	if b == nil {
		return vfilter.Null{}, false
	}

	idx := 0
	switch t := b.(type) {
	case string:
		idx, _ = strconv.Atoi(t)
	case int:
		idx = int(t)
	case float64:
		idx = int(t)
	case uint64:
		idx = int(t)
	case int64:
		idx = int(t)
	case *int:
		idx = int(*t)
	case *float64:
		idx = int(*t)
	case *uint64:
		idx = int(*t)
	case *int64:
		idx = int(*t)

	default:
		return vfilter.Null{}, false
	}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	if a_value.Len() == 0 {
		return vfilter.Null{}, false
	}

	// Modulus for negative numbers should wrap around the length
	// of the array aka python style modulus
	// (http://python-history.blogspot.com/2010/08/why-pythons-integer-division-floors.html).
	// This way indexing negative indexes will count from the back
	// of the array.
	length := a_value.Len()
	idx = (idx%length + length) % length
	return a_value.Index(idx).Interface(), true
}

func (self _IndexAssociativeProtocol) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {
	return []string{}
}

type WriteJSONPluginArgs struct {
	Filename   *accessors.OSPath   `vfilter:"required,field=filename,doc=JSONL files to open"`
	Accessor   string              `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Query      vfilter.StoredQuery `vfilter:"required,field=query,doc=query to write into the file."`
	BufferSize int                 `vfilter:"optional,field=buffer_size,doc=Maximum size of buffer before flushing to file."`
	MaxTime    int                 `vfilter:"optional,field=max_time,doc=Maximum time before flushing the buffer (10 sec)."`
	Append     bool                `vfilter:"optional,field=append,doc=Append JSONL records to existing file."`
}

type WriteJSONPlugin struct{}

func (self WriteJSONPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "write_jsonl", args)()

		arg := &WriteJSONPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("write_jsonl: %s", err.Error())
			return
		}

		if arg.BufferSize == 0 {
			arg.BufferSize = BUFF_SIZE
		}

		max_time := 10 * time.Second
		if arg.MaxTime > 0 {
			max_time = time.Duration(arg.MaxTime) * time.Second
		}

		open_options := os.O_RDWR | os.O_CREATE | os.O_TRUNC
		if arg.Append {
			open_options = os.O_RDWR | os.O_CREATE | os.O_APPEND
		}

		var writer *bufio.Writer

		switch arg.Accessor {
		case "", "auto", "file":
			err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
			if err != nil {
				scope.Log("write_jsonl: %s", err)
				return
			}

			// Make sure we are allowed to write there.
			err = file.CheckPrefix(arg.Filename)
			if err != nil {
				scope.Log("write_jsonl: %v", err)
				return
			}

			underlying_file, err := accessors.GetUnderlyingAPIFilename(
				arg.Accessor, scope, arg.Filename)
			if err != nil {
				scope.Log("write_jsonl: %s", err)
				return
			}

			file, err := os.OpenFile(underlying_file, open_options, 0700)
			if err != nil {
				scope.Log("write_jsonl: Unable to open file %s: %s",
					arg.Filename, err.Error())
				return
			}
			defer file.Close()

			writer = bufio.NewWriterSize(file, arg.BufferSize)
			defer writer.Flush()

		default:
			scope.Log("write_jsonl: Unsupported accessor for writing %v", arg.Accessor)
			return
		}

		lf := []byte("\n")

		events_chan := arg.Query.Eval(ctx, scope)

		for {
			select {
			case <-ctx.Done():
				return

			case <-utils.GetTime().After(max_time):
				writer.Flush()

			case row, ok := <-events_chan:
				if !ok {
					return
				}

				serialized, err := json.Marshal(row)
				if err == nil {
					_, _ = writer.Write(serialized)
					_, _ = writer.Write(lf)
				}

				select {
				case <-ctx.Done():
					return

				case output_chan <- row:
				}
			}
		}
	}()

	return output_chan
}

func (self WriteJSONPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "write_jsonl",
		Doc:      "Write a query into a JSONL file.",
		ArgType:  type_map.AddType(scope, &WriteJSONPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

type WatchJsonlPlugin struct{}

func (self WatchJsonlPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "watch_jsonl",
		Doc:      "Watch a jsonl file and stream events from it.",
		ArgType:  type_map.AddType(scope, &syslog.ScannerPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func (self WatchJsonlPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "watch_jsonl", args)()

		arg := &syslog.ScannerPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_jsonl: %v", err)
			return
		}

		// This plugin needs to be running on clients which have no
		// server config object.
		client_config_obj, ok := artifacts.GetConfig(scope)
		if !ok {
			scope.Log("watch_jsonl: unable to get config")
			return
		}

		config_obj := &config_proto.Config{Client: client_config_obj}

		event_channel := make(chan vfilter.Row)

		// Register the output channel as a listener to the
		// global event.
		for _, filename := range arg.Filenames {
			cancel := syslog.GlobalSyslogService(config_obj).Register(
				filename, arg.Accessor, ctx, scope,
				event_channel)

			defer cancel()
		}

		// Wait until the query is complete.
		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-event_channel:
				if !ok {
					return
				}

				// Get the line from the event
				line := vql_subsystem.GetStringFromRow(scope, event, "Line")
				if line == "" {
					continue
				}

				json_event, err := utils.ParseJsonToObject([]byte(line))
				if err != nil {
					scope.Log("Invalid jsonl: %v\n", line)
					continue
				}

				select {
				case <-ctx.Done():
					return

				case output_chan <- json_event:
				}
			}
		}
	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterFunction(&ParseJsonFunction{})
	vql_subsystem.RegisterFunction(&ParseJsonArray{})
	vql_subsystem.RegisterProtocol(&_nilAssociativeProtocol{})
	vql_subsystem.RegisterProtocol(&_MapInterfaceAssociativeProtocol{})
	vql_subsystem.RegisterProtocol(&_ProtobufAssociativeProtocol{})
	vql_subsystem.RegisterProtocol(&_IndexAssociativeProtocol{})
	vql_subsystem.RegisterPlugin(&ParseJsonArrayPlugin{})
	vql_subsystem.RegisterPlugin(&ParseJsonlPlugin{})
	vql_subsystem.RegisterPlugin(&WriteJSONPlugin{})
	vql_subsystem.RegisterPlugin(&WatchJsonlPlugin{})
}
