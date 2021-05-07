// +build windows

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

// Get Authenticode information from signed binaries. Currently only
// using the windows API.
package authenticode

// #cgo LDFLAGS: -lcrypt32 -lwintrust
// #include "authenticode.h"
import "C"

import (
	"context"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows/filesystems"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type AuthenticodeArgs struct {
	Filename string `vfilter:"required,field=filename,doc=The filename to parse."`
}

type AuthenticodeFunction struct{}

func (self *AuthenticodeFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("authenticode: %s", err)
		return vfilter.Null{}
	}

	arg := &AuthenticodeArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("authenticode: %v", err)
		return vfilter.Null{}
	}

	// We need to pass the actual file to the
	// WinAPI. Unfortunately using this method we are unable to
	// specify a different accessor.
	filename, err := windows.UTF16FromString(filesystems.GetPath(arg.Filename))
	if err != nil {
		scope.Log("authenticode: %v", err)
		return vfilter.Null{}
	}

	data := C.authenticode_data_struct{}
	defer C.free_authenticode_data_struct(&data)

	C.verify_file_authenticode((*C.wchar_t)(&filename[0]), &data)

	return ordereddict.NewDict().
		Set("Filename", _WCharToString(data.filename)).
		Set("ProgramName", _WCharToString(data.program_name)).
		Set("PublisherLink", _WCharToString(data.publisher_link)).
		Set("MoreInfoLink", _WCharToString(data.more_info_link)).
		Set("SerialNumber", C.GoString(data.signer_cert_serial_number)).
		Set("IssuerName", C.GoString(data.issuer_name)).
		Set("SubjectName", C.GoString(data.subject_name)).
		Set("TimestampIssuerName", C.GoString(data.timestamp_issuer_name)).
		Set("TimestampSubjectName", C.GoString(data.timestamp_subject_name)).
		Set("Timestamp", C.GoString(data.timestamp)).
		Set("Trusted", C.GoString(data.trusted))
}

func (self AuthenticodeFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "authenticode",
		Doc: "This plugin uses the Windows API to extract authenticode " +
			"signature details from PE files.",
		ArgType: type_map.AddType(scope, &AuthenticodeArgs{}),
	}
}

// I would really love to have these functions in a common library but
// unfortunately Cgo functions can not live in a separate package
// since they create private types. So we have to redefine the same
// code in each module that uses cgo.
func _WCharToString(ptr *C.ushort) string {
	p := (*[0xffff]uint16)(unsafe.Pointer(ptr))
	if p == nil {
		return ""
	}

	return windows.UTF16ToString(p[:])
}

func init() {
	vql_subsystem.RegisterFunction(&AuthenticodeFunction{})
}
