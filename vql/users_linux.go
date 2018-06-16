//
package vql

import (
	"bytes"
	"os"
	"www.velocidex.com/golang/velociraptor/binary"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/vfilter"
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

func extractUserRecords(
	scope *vfilter.Scope,
	args *vfilter.Dict) []vfilter.Row {
	var result []vfilter.Row
	filename := "/var/log/wtmp"
	arg, pres := args.Get("filename")
	if pres {
		filename, _ = arg.(string)
	}

	file, err := os.Open(filename)
	if err != nil {
		return result
	}
	defer file.Close()

	profile := binary.NewProfile()
	binary.AddModel(profile)

	err = profile.ParseStructDefinitions(UTMP_PROFILE)
	if err != nil {
		return result
	}

	// We make a copy of the data to avoid race
	// conditions. Otherwise we might close the file before
	// VFilter finishes analyzing the returned object and might
	// require a new read. This only works because there are no
	// free pointers.
	for {
		buf := make([]byte, profile.StructSize("utmp", 0, file))
		_, err := file.Read(buf)
		if err != nil {
			break
		}
		reader := bytes.NewReader(buf)
		obj := profile.Create("utmp", 0, reader)
		result = append(result, obj)
	}

	return result
}

func MakeUsersPlugin() vfilter.GenericListPlugin {
	return vfilter.GenericListPlugin{
		PluginName: "users",
		Function:   extractUserRecords,
	}
}
