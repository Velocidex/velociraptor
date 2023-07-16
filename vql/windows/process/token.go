// +build windows,amd64

package process

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type TokenArgs struct {
	Pid int64 `vfilter:"required,field=pid,doc=The PID to get the token for."`
}

type TokenFunction struct{}

func (self TokenFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("token: %s", err)
		return vfilter.Null{}
	}

	arg := &TokenArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("token: %s", err.Error())
		return vfilter.Null{}
	}
	handle, err := windows.OpenProcess(
		syscall.PROCESS_QUERY_INFORMATION, false, uint32(arg.Pid))

	if err != nil {
		scope.Log("token: %s", err.Error())
		return vfilter.Null{}
	}
	defer windows.CloseHandle(handle)

	var token windows.Token

	// Find process token via win32
	err = windows.OpenProcessToken(handle, syscall.TOKEN_QUERY, &token)
	if err != nil {
		scope.Log("token: %s", err.Error())
		return vfilter.Null{}
	}
	defer token.Close()

	// Find the token user
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		scope.Log("token: %s", err.Error())
		return vfilter.Null{}
	}

	groups := []string{}
	token_groups, err := token.GetTokenGroups()
	if err == nil {
		for _, grp := range token_groups.AllGroups() {
			group_name := grp.Sid.String()
			groups = append(groups, group_name)
		}
	}

	result := ordereddict.NewDict().
		Set("Username", vfilter.Null{}).
		Set("ProfileDir", vfilter.Null{}).
		Set("IsElevated", token.IsElevated()).
		Set("Groups", groups).
		Set("SID", tokenUser.User.Sid.String()).
		Set("Privileges", vfilter.Null{}).
		Set("PrimaryGroup", vfilter.Null{})

	// look up domain account by sid
	account, domain, _, err := tokenUser.User.Sid.LookupAccount("localhost")
	if err == nil {
		result.Update("Username", fmt.Sprintf("%s\\%s", domain, account))
	}

	profile_dir, err := token.GetUserProfileDirectory()
	if err == nil {
		result.Update("ProfileDir", profile_dir)
	}
	pg, err := token.GetTokenPrimaryGroup()
	if err == nil {
		str := pg.PrimaryGroup.String()
		result.Update("PrimaryGroup", str)
	}

	// Get privileges if possible
	privs, err := getTokenPrivileges(token)
	if err == nil {
		utils.Debug(privs)
		result.Update("Privileges", privs)
	}

	return result
}

func getTokenPrivileges(t windows.Token) (*ordereddict.Dict, error) {
	n := uint32(1024)
	for {
		b := make([]byte, n)
		e := windows.GetTokenInformation(t, windows.TokenPrivileges, &b[0], uint32(len(b)), &n)
		if n < 4 {
			return nil, errors.New("GetTokenInformation call too small!")
		}

		if e == nil {
			parsed := (*windows.Tokenprivileges)(unsafe.Pointer(&b[0]))
			if parsed.PrivilegeCount < 1024 {
				result := ordereddict.NewDict()
				for _, item := range parsed.AllPrivileges() {
					utils.Debug(item)
				}
				return result, nil
			}
			return nil, nil
		}
		if e != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil, e
		}
		if n <= uint32(len(b)) {
			return nil, e
		}
	}
}

func (self TokenFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "token",
		Doc:      "Extract process token.",
		ArgType:  type_map.AddType(scope, &TokenArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&TokenFunction{})
}
