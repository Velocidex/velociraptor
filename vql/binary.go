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
// VQL bindings to binary parsing.
package vql

import (
	"context"
	"io"
	"strings"

	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vtypes"
)

type _binaryFieldImpl struct{}

func (self _binaryFieldImpl) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, b_ok := b.(string)
	switch a.(type) {
	case vtypes.BaseObject, *vtypes.BaseObject:
		return b_ok
	}
	return false
}

func (self _binaryFieldImpl) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	field := b.(string)

	var res vtypes.Object

	switch t := a.(type) {
	case vtypes.BaseObject:
		res = t.Get(field)
	case *vtypes.BaseObject:
		res = t.Get(field)
	default:
		return nil, false
	}

	// If the resolving returns an error object we have not
	// properly resolved the field.
	_, ok := res.(*vtypes.ErrorObject)
	if ok {
		// Try to resolve using the default associative for
		// methods.
		return vfilter.DefaultAssociative{}.Associative(scope, a, b)
	}

	return res, true
}

func (self _binaryFieldImpl) GetMembers(scope *vfilter.Scope, a vfilter.Any) []string {
	switch t := a.(type) {
	case vtypes.BaseObject:
		return t.Fields()
	case *vtypes.BaseObject:
		return t.Fields()
	default:
		return []string{}
	}
}

type _BinaryParserPluginArg struct {
	Offset     int64       `vfilter:"optional,field=offset,doc=Start parsing from this offset"`
	File       string      `vfilter:"required,field=file,doc=Filename to parse"`
	Accessor   string      `vfilter:"optional,field=accessor,doc=Accessor to use (e.g. ntfs, data)"`
	Profile    string      `vfilter:"optional,field=profile,doc=Profile to use."`
	Target     string      `vfilter:"required,field=target,doc=The target to fetch."`
	Args       vfilter.Any `vfilter:"optional,field=args,doc=Args for the target class."`
	StartField string      `vfilter:"optional,field=start,doc=The initial field in the target to fetch."`
}

type _BinaryParserPlugin struct{}

func (self _BinaryParserPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	arg := &_BinaryParserPluginArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("%s: %s", self.Name(), err.Error())
		close(output_chan)
		return output_chan
	}

	// Extract additional args
	options := make(map[string]interface{})
	if arg.Args != nil {
		for _, k := range scope.GetMembers(arg.Args) {
			v, pres := scope.Associative(arg.Args, k)
			if pres {
				options[k] = v
			}
		}
	}

	go func() {
		defer close(output_chan)

		var file io.Reader
		if arg.File != "" {
			accessor := arg.Accessor
			if accessor == "" {
				accessor = "file"
			}
			file_handle, err := glob.GetAccessor(accessor, ctx).Open(arg.File)
			if err != nil {
				scope.Log("%s: %s", self.Name(), err.Error())
				return
			}

			var ok bool
			file, ok = file_handle.(io.Reader)
			if !ok {
				return
			}
		} else {
			scope.Log("%s: %s", self.Name(), "At least on of file or string must be given.")
			return

		}
		// Only close the file when the context (and the VQL
		// query) is fully done because we are releasing
		// objects which may reference the file. These objects
		// may participate in WHERE clause and so will be
		// referenced after the plugin is terminated.

		// This is a real bad strategy. We should ensure that
		// we are taking a copy of file content here! This
		// leaks if the VQL is long.
		go func() {
			<-ctx.Done()
			fd, ok := file.(io.Closer)
			if ok {
				fd.Close()
			}
		}()

		profile := vtypes.NewProfile()
		vtypes.AddModel(profile)

		err = profile.ParseStructDefinitions(arg.Profile)
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return
		}

		reader, ok := file.(io.ReaderAt)
		if !ok {
			scope.Log("%s: file is not seekable", self.Name())
			return
		}

		target, err := profile.Create(
			arg.Target, arg.Offset, reader, options)
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return
		}

		array := target
		if arg.StartField != "" {
			array = target.Get(arg.StartField)
		}
		if ok {
			for {
				value := array.Next()
				if !value.IsValid() {
					break
				}

				output_chan <- value
			}
		}
	}()

	return output_chan
}

func (self _BinaryParserPlugin) Name() string {
	return "binary_parse"
}

func (self _BinaryParserPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "binary_parse",
		Doc:     "Parse binary files using a profile.",
		ArgType: type_map.AddType(scope, &_BinaryParserPluginArg{}),
	}
}

type _BinaryParserFunctionArg struct {
	Offset   int64  `vfilter:"optional,field=offset,doc=Start parsing from this offset."`
	String   string `vfilter:"required,field=string,doc=The string to parse."`
	Profile  string `vfilter:"optional,field=profile,doc=The profile to use."`
	Iterator string `vfilter:"optional,field=iterator,doc=An iterator to begin with."`
	Target   string `vfilter:"optional,field=target,doc=The target type to fetch."`
}

type _BinaryParserFunction struct{}

func (self *_BinaryParserFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	result := []vfilter.Row{}

	arg := &_BinaryParserFunctionArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("binary_parse: %s", err.Error())
		return vfilter.Null{}
	}

	// Extract additional args
	options := make(map[string]interface{})
	for k, v := range *args.ToDict() {
		switch k {
		case "offset", "string", "profile", "iterator", "accessor", "target":
			continue
		default:
			options[k] = v
		}
	}

	profile := vtypes.NewProfile()
	vtypes.AddModel(profile)

	if arg.Profile != "" {
		err = profile.ParseStructDefinitions(arg.Profile)
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return result
		}
	}
	reader := strings.NewReader(arg.String)
	if arg.Iterator != "" {
		array, err := profile.Create(
			arg.Iterator, arg.Offset, reader, options)
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return result
		}
		for {
			value := array.Next()
			if !value.IsValid() {
				break
			}

			result = append(result, value)
		}
		return result

	} else if arg.Target != "" {
		target, err := profile.Create(
			arg.Target, arg.Offset, reader, options)

		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return result
		}

		return target
	}

	return vfilter.Null{}
}

func (self _BinaryParserFunction) Name() string {
	return "binary_parse"
}

func (self _BinaryParserFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "binary_parse",
		Doc:     "Parse a binary string with profile based parser.",
		ArgType: type_map.AddType(scope, &_BinaryParserFunctionArg{}),
	}
}

func init() {
	RegisterProtocol(&_binaryFieldImpl{})
	RegisterFunction(&_BinaryParserFunction{})
	RegisterPlugin(&_BinaryParserPlugin{})
}
