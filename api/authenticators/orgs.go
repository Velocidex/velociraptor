package authenticators

import (
	"errors"
	"net/http"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func CheckOrgAccess(r *http.Request, user_record *api_proto.VelociraptorUser) error {

	// Now we have to determine which org the user wants to
	// use. First let's check if the user specified an org in the
	// header.
	org_id := "root"
	current_orgid_array := r.Header.Get("Grpc-Metadata-Orgid")
	if len(current_orgid_array) == 1 {
		org_id = string(current_orgid_array[0])
	}

	err := _checkOrgAccess(r, org_id, user_record)
	if err == nil {
		return nil
	}

	// Redirect the user to the first org they have access to
	for _, org := range user_record.Orgs {
		err := _checkOrgAccess(r, org.Id, user_record)
		if err == nil {
			r.Header.Set("Grpc-Metadata-Orgid", org.Id)

			// Update the user's org preferences
			user_manager := services.GetUserManager()
			user_options, err := user_manager.GetUserOptions(user_record.Name)
			if err == nil {
				user_options.Org = org.Id
			} else {
				user_options = &api_proto.SetGUIOptionsRequest{
					Org: org.Id,
				}
			}
			user_manager.SetUserOptions(user_record.Name, user_options)

			return nil
		}
	}

	return errors.New("Unauthorized username")
}

func _checkOrgAccess(r *http.Request, org_id string, user_record *api_proto.VelociraptorUser) error {
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	org_config_obj, err := org_manager.GetOrgConfig(org_id)
	if err != nil {
		return err
	}

	perm, err := acls.CheckAccess(org_config_obj, user_record.Name, acls.READ_RESULTS)
	if err != nil {
		return err
	}

	if !perm || user_record.Locked {
		return errors.New("Unauthorized username")
	}

	return nil
}
