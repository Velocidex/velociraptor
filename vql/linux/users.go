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
//
package linux

import (
	_ "bytes"
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vtypes"
)

var (
	// This is automatically generated from dwarf symbols:
	// gcc -c -g -o /tmp/test.o /tmp/1.c
	// rekall dwarfparser /tmp/test.o

	// And 1.c is:
	// #include "utmp.h"
	// struct utmp x;
	UTMP_PROFILE string = `
{
  "timeval": [8, {
   "tv_sec": [0, ["int"]],
   "tv_usec": [4, ["int"]]
  }],
  "exit_status": [4, {
   "e_exit": [2, ["short int"]],
   "e_termination": [0, ["short int"]]
  }],
  "timezone": [8, {
   "tz_dsttime": [4, ["int"]],
   "tz_minuteswest": [0, ["int"]]
  }],
  "utmp": [384, {
   "__glibc_reserved": [364, ["Array", {
    "count": 20,
    "target": "char",
    "target_args": null
   }]],
   "ut_addr_v6": [348, ["Array", {
    "count": 4,
    "target": "int",
    "target_args": null
   }]],
   "ut_exit": [332, ["exit_status"]],
   "ut_host": [76, ["String", {
    "length": 256
   }]],
   "ut_id": [40, ["String", {
    "length": 4
   }]],
   "ut_line": [8, ["String", {
    "length": 32
   }]],
   "ut_pid": [4, ["int"]],
   "ut_session": [336, ["int"]],
   "ut_tv": [340, ["timeval"]],
   "ut_type": [0, ["Enumeration", {
     "target": "short int",
     "choices": {
        "0": "EMPTY",
        "1": "RUN_LVL",
        "2": "BOOT_TIME",
        "5": "INIT_PROCESS",
        "6": "LOGIN_PROCESS",
        "7": "USER_PROCESS",
        "8": "DEAD_PROCESS"
      }
   }]],
   "ut_user": [44, ["String", {
    "length": 32
   }]]
  }]
}
`
)

type _UsersPluginArg struct {
	File string `vfilter:"optional,field=file,doc=The wtmp file to use"`
}

type _UsersPlugin struct{}

func (self _UsersPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("users: %s", err)
			return
		}

		arg := &_UsersPluginArg{}
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("%s: %s", "users", err.Error())
			return
		}

		// Default location.
		if arg.File == "" {
			arg.File = "/var/log/wtmp"
		}

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
			<-ctx.Done()
			file.Close()
		}()

		profile := vtypes.NewProfile()
		vtypes.AddModel(profile)

		err = profile.ParseStructDefinitions(UTMP_PROFILE)
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
			select {
			case <-ctx.Done():
				return
			case output_chan <- value:
			}
		}
	}()

	return output_chan
}

func (self _UsersPlugin) Name() string {
	return "users"
}

func (self _UsersPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "users",
		Doc:     "List last logged in users based on wtmp records.",
		ArgType: type_map.AddType(scope, &_UsersPluginArg{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_UsersPlugin{})
}
