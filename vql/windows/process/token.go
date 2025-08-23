//go:build windows && amd64
// +build windows,amd64

package process

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vwindows "www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	luid_resolver = LuidResolver{
		lookup: make(map[uint64]string),
	}
)

type TokenArgs struct {
	Pid int64 `vfilter:"required,field=pid,doc=The PID to get the token for."`
}

type TokenFunction struct{}

func (self TokenFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "token", args)()

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

	TryToGrantSeDebugPrivilege()

	handle, err := windows.OpenProcess(
		syscall.PROCESS_QUERY_INFORMATION, false, uint32(arg.Pid))

	if err != nil {
		scope.Log("token: OpenProcess for %v: %s",
			GetProcessContext(ctx, scope, uint64(arg.Pid)), err.Error())
		return vfilter.Null{}
	}
	defer windows.CloseHandle(handle)

	var token windows.Token

	// Find process token via win32
	err = windows.OpenProcessToken(handle, syscall.TOKEN_QUERY, &token)
	if err != nil {
		scope.Log("token: OpenProcessToken for %v: %s",
			GetProcessContext(ctx, scope, uint64(arg.Pid)), err.Error())
		return vfilter.Null{}
	}
	defer token.Close()

	// Find the token user
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		scope.Log("token: GetTokenUser for %v: %s",
			GetProcessContext(ctx, scope, uint64(arg.Pid)), err.Error())
		return vfilter.Null{}
	}

	groups := ordereddict.NewDict()
	token_groups, err := token.GetTokenGroups()
	if err == nil {
		for _, grp := range token_groups.AllGroups() {
			group_name := grp.Sid.String()
			access := []string{}
			if grp.Attributes&windows.SE_GROUP_ENABLED > 0 {
				access = append(access, "ENABLED")
			}
			if grp.Attributes&windows.SE_GROUP_ENABLED_BY_DEFAULT > 0 {
				access = append(access, "ENABLED_BY_DEFAULT")
			}
			if grp.Attributes&windows.SE_GROUP_INTEGRITY > 0 {
				access = append(access, "INTEGRITY")
			}
			if grp.Attributes&windows.SE_GROUP_INTEGRITY_ENABLED > 0 {
				access = append(access, "INTEGRITY_ENABLED")
			}
			if grp.Attributes&windows.SE_GROUP_LOGON_ID > 0 {
				access = append(access, "LOGON_ID")
			}
			if grp.Attributes&windows.SE_GROUP_MANDATORY > 0 {
				access = append(access, "MANDATORY")
			}
			if grp.Attributes&windows.SE_GROUP_OWNER > 0 {
				access = append(access, "OWNER")
			}
			if grp.Attributes&windows.SE_GROUP_RESOURCE > 0 {
				access = append(access, "RESOURCE")
			}
			if grp.Attributes&windows.SE_GROUP_USE_FOR_DENY_ONLY > 0 {
				access = append(access, "USE_FOR_DENY_ONLY")
			}
			if len(access) > 0 {
				groups.Set(group_name, strings.Join(access, ","))
			}
		}
	}

	result := ordereddict.NewDict().
		Set("Username", vfilter.Null{}).
		Set("ProfileDir", vfilter.Null{}).
		Set("IsElevated", token.IsElevated()).
		Set("Groups", groups).
		Set("GroupNames", func() vfilter.Any {
			result := ordereddict.NewDict()
			for _, i := range groups.Items() {
				result.Set(vwindows.GetNameFromSID(i.Key), i.Value)
			}
			return result
		}).
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
		result.Update("Privileges", privs)
	}

	return result
}

func getTokenPrivileges(t windows.Token) (*ordereddict.Dict, error) {
	n := uint32(1024)
	for {
		b := utils.AllocateBuff(int(n))
		e := windows.GetTokenInformation(t, windows.TokenPrivileges, &b[0], uint32(len(b)), &n)
		if n < 4 {
			return nil, errors.New("GetTokenInformation call too small!")
		}

		if e == nil {
			parsed := (*windows.Tokenprivileges)(unsafe.Pointer(&b[0]))
			if parsed.PrivilegeCount < 1024 {
				result := ordereddict.NewDict()
				for _, luid_attr := range parsed.AllPrivileges() {
					name := luid_resolver.Lookup(
						luid_attr.Luid.LowPart, luid_attr.Luid.HighPart)
					access := []string{}
					if luid_attr.Attributes&windows.SE_PRIVILEGE_ENABLED > 0 {
						access = append(access, "ENABLED")
					}

					if luid_attr.Attributes&windows.SE_PRIVILEGE_ENABLED_BY_DEFAULT > 0 {
						access = append(access, "ENABLED_BY_DEFAULT")
					}

					if luid_attr.Attributes&windows.SE_PRIVILEGE_REMOVED > 0 {
						access = append(access, "REMOVED")
					}

					if luid_attr.Attributes&windows.SE_PRIVILEGE_USED_FOR_ACCESS > 0 {
						access = append(access, "USED_FOR_ACCESS")
					}

					// Only include privileges that are set to
					// something.
					if len(access) > 0 {
						result.Set(name, strings.Join(access, ","))
					}
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

// Cache Luid to avoid having to make too many API calls
type LuidResolver struct {
	mu     sync.Mutex
	lookup map[uint64]string
}

func (self *LuidResolver) Lookup(
	low uint32, high int32) string {
	self.mu.Lock()
	defer self.mu.Unlock()

	key := uint64(high)<<32 | uint64(low)

	res, ok := self.lookup[key]
	if ok {
		return res
	}

	// Get the name of the privilege
	b := make([]uint16, 50)
	n := uint32(50)
	luid := vwindows.LUID{low, high}
	err := vwindows.LookupPrivilegeName(
		nil, &luid, &b[0], &n)
	if err == nil {
		res = windows.UTF16ToString(b[:n])
	}

	self.lookup[key] = res
	return res
}
