package acls

/*

ACLs enforce access to the api clients.

API Clients are created by using the "config api_client" command -
this generates a certificate with a common name. This common name is
associated with the particular program which uses the api_client
certificate.

The ACL system attaches a policy to users of the api client. Before
the Velociraptor server executes an action from an API client, the
client's ACL policy is retrieved and the action is checked against it.

*/

import (
	"path"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
)

type ACL_PERMISSION int

const (
	QUERY   ACL_PERMISSION = iota
	PUBLISH                = iota
)

func GetPolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	acl_obj := &acl_proto.ApiClientACL{}
	err = db.GetSubject(config_obj,
		path.Join("acl", principal+".json"), acl_obj)
	if err != nil {
		return nil, err
	}
	return acl_obj, nil
}

func SetPolicy(
	config_obj *config_proto.Config,
	principal string, acl_obj *acl_proto.ApiClientACL) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	return db.SetSubject(config_obj,
		path.Join("acl", principal+".json"), acl_obj)
}

func CheckAccess(
	config_obj *config_proto.Config,
	principal string,
	permission ACL_PERMISSION, args ...string) (bool, error) {

	acl_obj, err := GetPolicy(config_obj, principal)
	if err != nil {
		return false, err
	}

	// Requested permission
	switch permission {
	case QUERY:
		// Principal is allowed all queries.
		if acl_obj.AllQuery {
			return true, nil
		}

	case PUBLISH:
		if len(args) == 1 {
			for _, allowed_queue := range acl_obj.PublishQueues {
				if allowed_queue == args[0] {
					return true, nil
				}

			}
		}
	}

	return false, nil
}
