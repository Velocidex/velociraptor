package authenticators

import (
	"errors"
	"net/http"
	"net/url"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func GetOrgIdFromRequest(r *http.Request) string {
	// Now we have to determine which org the user wants to use. First
	// let's check if the user specified an org in the header.
	org_id := r.Header.Get("Grpc-Metadata-Orgid")
	if org_id != "" {
		return org_id
	}

	// Maybe the org id is specified in the URL itself. We allow
	// the org id to be specified as a query string in order to
	// support plain href links. However ultimately the GRPC
	// gateway needs to check the org id in a header - so if an
	// org is specified using a query string and NOT specified
	// using a header, we set the header from it for further
	// checks by the GRPC layer (in services/users/grpc.go)
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err == nil {
		org_id = q.Get("org_id")
		if org_id != "" {
			r.Header.Set("Grpc-Metadata-Orgid", org_id)
			return org_id
		}
	}

	org_id = "root"
	r.Header.Set("Grpc-Metadata-Orgid", org_id)
	return org_id
}

func CheckOrgAccess(r *http.Request, user_record *api_proto.VelociraptorUser) error {
	org_id := GetOrgIdFromRequest(r)
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
			user_options, err := user_manager.GetUserOptions(
				r.Context(), user_record.Name)
			if err == nil {
				user_options.Org = org.Id
			} else {
				user_options = &api_proto.SetGUIOptionsRequest{
					Org: org.Id,
				}
			}
			user_manager.SetUserOptions(
				r.Context(), user_record.Name, user_options)

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
