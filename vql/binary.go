// VQL bindings to binary parsing.
package vql

import (
	"context"
	"os"
	"www.velocidex.com/golang/velociraptor/binary"
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
	Offset   uint64 `vfilter:"optional,field=offset"`
	File     string `vfilter:"required,field=file"`
	Profile  string `vfilter:"required,field=profile"`
	Iterator string `vfilter:"required,field=iterator"`
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
	for k, v := range *args.ToDict() {
		switch k {
		case "offset", "file", "profile", "iterator":
			continue
		default:
			options[k] = v
		}
	}

	go func() {
		defer close(output_chan)
		file, err := os.Open(arg.File)
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
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
					file.Close()
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

		options := make(map[string]interface{})
		options["Target"] = "utmp"
		array, err := profile.Create("Array", 0, file, options)
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return
		}
		for {
			value := array.Next()
			if !value.IsValid() {
				break
			}

			output_chan <- value
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
		Doc:     "List last logged in users based on wtmp records.",
		ArgType: type_map.AddType(&_BinaryParserPluginArg{}),
	}
}

func init() {
	RegisterProtocol(&_binaryFieldImpl{})
	RegisterPlugin(&_BinaryParserPlugin{})
}
