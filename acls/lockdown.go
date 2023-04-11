package acls

import (
	"sync"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
)

var (
	mu             sync.Mutex
	lockdown_token *acl_proto.ApiClientACL
)

func LockdownToken() *acl_proto.ApiClientACL {
	mu.Lock()
	defer mu.Unlock()
	return lockdown_token
}

func SetLockdownToken(token *acl_proto.ApiClientACL) {
	mu.Lock()
	defer mu.Unlock()
	lockdown_token = token
}
