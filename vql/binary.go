// VQL bindings to binary parsing.
package vql

import (
	"context"
	"io"
	"strings"

	"www.velocidex.com/golang/velociraptor/binary"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type _binaryFieldImpl struct{}

func (self _binaryFieldImpl) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, b_ok := b.(string)
	switch a.(type) {
	case binary.BaseObject, *binary.BaseObject:
		return b_ok
	}
	return false
}

func (self _binaryFieldImpl) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	field := b.(string)

	var res binary.Object

	switch t := a.(type) {
	case binary.BaseObject:
		res = t.Get(field)
	case *binary.BaseObject:
		res = t.Get(field)
	default:
		return nil, false
	}

	// If the resolving returns an error object we have not
	// properly resolved the field.
	_, ok := res.(*binary.ErrorObject)
	if ok {
		return nil, false
	}

	return res.Value(), true
}

func (self _binaryFieldImpl) GetMembers(scope *vfilter.Scope, a vfilter.Any) []string {
	switch t := a.(type) {
	case binary.BaseObject:
		return t.Fields()
	case *binary.BaseObject:
		return t.Fields()
	default:
		return []string{}
	}
}

type _BinaryParserPluginArg struct {
	Offset     int64       `vfilter:"optional,field=offset"`
	File       string      `vfilter:"optional,field=file"`
	String     string      `vfilter:"optional,field=string"`
	Accessor   string      `vfilter:"optional,field=accessor"`
	Profile    string      `vfilter:"required,field=profile"`
	Target     string      `vfilter:"required,field=target"`
	Args       vfilter.Any `vfilter:"optional,field=args"`
	StartField string      `vfilter:"optional,field=start"`
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
			file_handle, err := glob.GetAccessor(accessor).Open(arg.File)
			if err != nil {
				scope.Log("%s: %s", self.Name(), err.Error())
				return
			}

			var ok bool
			file, ok = file_handle.(io.Reader)
			if !ok {
				return
			}
		} else if arg.String != "" {
			file = strings.NewReader(arg.String)
		} else {
			scope.Log("%s: %s", self.Name(), "At least on of file or string must be given.")
			return

		}
		// Only close the file when the context (and the VQL
		// query) is fully done because we are releasing
		// objects which may reference the file. These objects
		// may participate in WHERE clause and so will be
		// referenced after the plugin is terminated.
		go func() {
			for {
				select {
				case <-ctx.Done():
					fd, ok := file.(io.Closer)
					if ok {
						fd.Close()
					}
					return
				}
			}
		}()

		profile := binary.NewProfile()
		binary.AddModel(profile)

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

func (self _BinaryParserPlugin) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "binary_parse",
		Doc:     "Parse binary files using a profile.",
		ArgType: type_map.AddType(&_BinaryParserPluginArg{}),
	}
}

type _BinaryParserFunctionArg struct {
	Offset   int64  `vfilter:"optional,field=offset"`
	String   string `vfilter:"required,field=string"`
	Profile  string `vfilter:"required,field=profile"`
	Iterator string `vfilter:"required,field=iterator"`
	Accessor string `vfilter:"optional,field=accessor"`
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
		case "offset", "string", "profile", "iterator", "accessor":
			continue
		default:
			options[k] = v
		}
	}

	profile := binary.NewProfile()
	binary.AddModel(profile)

	err = profile.ParseStructDefinitions(arg.Profile)
	if err != nil {
		scope.Log("%s: %s", self.Name(), err.Error())
		return result
	}

	reader := strings.NewReader(arg.String)
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
}

func (self _BinaryParserFunction) Name() string {
	return "binary_parse"
}

func (self _BinaryParserFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "binary_parse",
		Doc:     "Parse a binary string with profile based parser.",
		ArgType: type_map.AddType(&_BinaryParserFunctionArg{}),
	}
}

func init() {
	RegisterProtocol(&_binaryFieldImpl{})
	RegisterFunction(&_BinaryParserFunction{})
	RegisterPlugin(&_BinaryParserPlugin{})
}
