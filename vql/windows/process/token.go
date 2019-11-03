// +build windows

package process

import (
	"context"
	"fmt"
	"syscall"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type TokenArgs struct {
	Pid int64 `vfilter:"required,field=pid,doc=The PID to get the token for."`
}

type TokenFunction struct{}

func (self TokenFunction) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &TokenArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
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

	// Find the token user
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		scope.Log("token: %s", err.Error())
		return vfilter.Null{}
	}
	defer token.Close()

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
		Set("PrimaryGroup", vfilter.Null{})

	// look up domain account by sid
	account, domain, _, err := tokenUser.User.Sid.LookupAccount("localhost")
	if err == nil {
		result.Set("Username", fmt.Sprintf("%s\\%s", domain, account))
	}

	profile_dir, err := token.GetUserProfileDirectory()
	if err == nil {
		result.Set("ProfileDir", profile_dir)
	}
	pg, err := token.GetTokenPrimaryGroup()
	if err == nil {
		str := pg.PrimaryGroup.String()
		result.Set("PrimaryGroup", str)
	}

	return result
}

func (self TokenFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "token",
		Doc:     "Extract process token.",
		ArgType: type_map.AddType(scope, &TokenArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&TokenFunction{})
}
