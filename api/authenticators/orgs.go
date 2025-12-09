package authenticators

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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

// Checks to make sure the user has access to the org they
// requested. If they do not have access to the org they requested we
// switch them to any org in which they have at least read
// access. This behaviour ensures that when a user's access is removed
// from an org the GUI immediately switches to the next available org.
func CheckOrgAccess(
	config_obj *config_proto.Config,
	r *http.Request,
	user_record *api_proto.VelociraptorUser,
	permission acls.ACL_PERMISSION) (err error) {

	org_id := GetOrgIdFromRequest(r)
	err = _checkOrgAccess(r, org_id, permission, user_record)
	if err == nil {
		return nil
	}

	// For the root org or an unknown org we switch to another org,
	// otherwise we need to give the user a more specific error that
	// they are not authorized for this org.
	if !utils.IsRootOrg(org_id) &&
		!errors.Is(err, services.OrgNotFoundError) &&
		!errors.Is(err, utils.NoAccessToOrgError) {
		return err
	}

	ctx := r.Context()

	// Does the user already have a preferred org they want to be in?
	user_manager := services.GetUserManager()
	user_options, err := user_manager.GetUserOptions(ctx, user_record.Name)
	if err != nil {
		// Not an error - maybe the user never logged in yet
		user_options = &api_proto.SetGUIOptionsRequest{}
	}

	// Ok they are allowed to go to their preferred org.
	err = _checkOrgAccess(r, user_options.Org, permission, user_record)
	if err == nil {
		r.Header.Set("Grpc-Metadata-Orgid", user_options.Org)

		// Log them into their org
		return user_manager.SetUserOptions(ctx, user_record.Name,
			user_record.Name, user_options)
	}

	// Redirect the user to the first org they have access to
	for _, org := range user_record.Orgs {
		err = _checkOrgAccess(r, org.Id, permission, user_record)
		if err == nil {
			r.Header.Set("Grpc-Metadata-Orgid", org.Id)

			// Log them into their org
			user_options.Org = org.Id
			return user_manager.SetUserOptions(ctx, user_record.Name,
				user_record.Name, user_options)
		}
	}

	if err != nil {
		return fmt.Errorf("Unable to access any orgs: %w", err)
	}

	return errors.New("Unauthorized username")
}

func _checkOrgAccess(r *http.Request,
	org_id string, permission acls.ACL_PERMISSION,
	user_record *api_proto.VelociraptorUser) error {
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	org_config_obj, err := org_manager.GetOrgConfig(org_id)
	if err != nil {
		return err
	}

	perm, err := services.CheckAccess(
		org_config_obj, user_record.Name, permission)
	if err != nil {
		return err
	}

	if !perm || user_record.Locked {
		return fmt.Errorf("User %v accessing %v: %w",
			user_record.Name, org_id, utils.NoAccessToOrgError)
	}

	return nil
}
