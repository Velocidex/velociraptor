//go:build windows
// +build windows

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

package windows

import (
	"context"
	"syscall"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserRecord struct {
	Name             string
	Password         string
	Password_age     int
	Priv             int
	Home_dir         string
	Comment          string
	Flags            int
	Script_path      string
	Auth_flags       int
	Full_name        string
	Usr_comment      string
	Parms            string
	Workstations     string
	Last_logon       int
	Last_logoff      int
	Acct_expires     int
	Max_storage      int
	Units_per_week   int
	Bad_pw_count     int
	Num_logons       int
	Logon_server     string
	Country_code     int
	Code_page        int
	User_id          int
	User_sid         string
	Primary_group_id int
	Profile          string
	Home_dir_drive   string
	Password_expired int
}

func ParseUserRecord(a *USER_INFO_3) *UserRecord {
	name := LPWSTRToString(a.Name)
	sid, _, _, _ := syscall.LookupSID("", name)
	sid_string, _ := sid.String()

	return &UserRecord{
		Name:             name,
		Password:         LPWSTRToString(a.Password),
		Password_age:     int(a.Password_age),
		Priv:             int(a.Priv),
		Home_dir:         LPWSTRToString(a.Home_dir),
		Comment:          LPWSTRToString(a.Comment),
		Flags:            int(a.Flags),
		Script_path:      LPWSTRToString(a.Script_path),
		Auth_flags:       int(a.Auth_flags),
		Full_name:        LPWSTRToString(a.Full_name),
		Usr_comment:      LPWSTRToString(a.Usr_comment),
		Parms:            LPWSTRToString(a.Parms),
		Workstations:     LPWSTRToString(a.Workstations),
		Last_logon:       int(a.Last_logon),
		Last_logoff:      int(a.Last_logoff),
		Acct_expires:     int(a.Acct_expires),
		Max_storage:      int(a.Max_storage),
		Units_per_week:   int(a.Units_per_week),
		Bad_pw_count:     int(a.Bad_pw_count),
		Num_logons:       int(a.Num_logons),
		Logon_server:     LPWSTRToString(a.Logon_server),
		Country_code:     int(a.Country_code),
		Code_page:        int(a.Code_page),
		User_id:          int(a.User_id),
		User_sid:         sid_string,
		Primary_group_id: int(a.Primary_group_id),
		Profile:          LPWSTRToString(a.Profile),
		Home_dir_drive:   LPWSTRToString(a.Home_dir_drive),
		Password_expired: int(a.Password_expired),
	}
}

func getUsers(
	ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row

	level := uint32(3)

	entriesread := uint32(0)
	totalentries := uint32(0)
	resume_handle := uint32(0)
	var buffer uintptr

	for {
		res := NetUserEnum(nil,
			level, FILTER_NORMAL_ACCOUNT|FILTER_WORKSTATION_TRUST_ACCOUNT,
			&buffer,
			uint32(0xFFFFFFFF),
			&entriesread, &totalentries, &resume_handle)
		defer NetApiBufferFree(buffer)

		if res == 0 {
			pos := buffer
			for i := uint32(0); i < entriesread; i++ {
				encoded_user_record := (*USER_INFO_3)(unsafe.Pointer(pos))
				if encoded_user_record == nil {
					scope.Log("Access denied when calling NetUserEnum.")
					break
				}
				user_record := ParseUserRecord(encoded_user_record)
				result = append(result, user_record)
				pos = pos + unsafe.Sizeof(*encoded_user_record)
			}
		}

		if res != ERROR_MORE_DATA {
			break
		}
	}
	return result
}

type LookupSidFunctionArgs struct {
	Sid string `vfilter:"required,field=sid,doc=A SID to lookup using LookupAccountSid "`
}

type LookupSidFunction struct{}

func (self *LookupSidFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "lookupSID", args)()

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("lookupSID: %s", err)
		return false
	}

	arg := &LookupSidFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("lookupSID: %s", err.Error())
		return false
	}

	return GetNameFromSID(arg.Sid)
}

func GetNameFromSID(name string) string {
	sid, err := syscall.StringToSid(name)
	if err != nil {
		return name
	}

	namelen := uint32(255)
	utf16_name := make([]uint16, namelen)
	sid_name_use := uint32(0)
	domain_len := uint32(255)
	domain := make([]uint16, domain_len)
	system_name := make([]uint16, 10)
	err = syscall.LookupAccountSid(&system_name[0], sid, &utf16_name[0], &namelen,
		&domain[0], &domain_len, &sid_name_use)
	if err != nil {
		return name
	}

	return syscall.UTF16ToString(utf16_name)
}

func (self *LookupSidFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "lookupSID",
		Doc:      "Get information about the SID.",
		ArgType:  type_map.AddType(scope, &LookupSidFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&vfilter.GenericListPlugin{
		PluginName: "users",
		Doc: "Display information about workstation local users. " +
			"This is obtained through the NetUserEnum() API.",
		Function: getUsers,
	})

	vql_subsystem.RegisterFunction(&LookupSidFunction{})
}
