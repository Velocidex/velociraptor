package api

import (
	"encoding/json"
	"io"
	"net/http"

	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

const maxJITBodySize = 65536 // 64KB

func readJITBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJITBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		returnError(w, 400, "Request body too large or unreadable")
		return nil, false
	}
	return body, true
}

func jitRequestRoleHandler() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				returnError(w, 405, "Method not allowed")
				return
			}

			org_id := authenticators.GetOrgIdFromRequest(r)
			org_id = utils.NormalizedOrgId(org_id)

			org_manager, err := services.GetOrgManager()
			if err != nil {
				returnError(w, 500, err.Error())
				return
			}

			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			user_record := GetUserInfo(r.Context(), org_config_obj)
			if user_record.Name == "" {
				returnError(w, 403, "Unauthenticated")
				return
			}

			body, ok := readJITBody(w, r)
			if !ok {
				return
			}

			request := &api_proto.JITRequestRoleRequest{}
			if err := json.Unmarshal(body, request); err != nil {
				returnError(w, 400, "Invalid request body")
				return
			}

			if request.OrgId == "" {
				request.OrgId = org_id
			}

			jit_manager, err := services.GetJITManager(org_config_obj)
			if err != nil {
				returnError(w, 500, "JIT service not available")
				return
			}

			result, err := jit_manager.RequestRole(
				org_config_obj, user_record.Name, request)
			if err != nil {
				returnError(w, 400, err.Error())
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})
}

func jitApproveHandler() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				returnError(w, 405, "Method not allowed")
				return
			}

			org_id := authenticators.GetOrgIdFromRequest(r)
			org_id = utils.NormalizedOrgId(org_id)

			org_manager, err := services.GetOrgManager()
			if err != nil {
				returnError(w, 500, err.Error())
				return
			}

			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			user_record := GetUserInfo(r.Context(), org_config_obj)
			if user_record.Name == "" {
				returnError(w, 403, "Unauthenticated")
				return
			}

			body, ok := readJITBody(w, r)
			if !ok {
				return
			}

			approval := &api_proto.JITApprovalRequest{}
			if err := json.Unmarshal(body, approval); err != nil {
				returnError(w, 400, "Invalid request body")
				return
			}

			jit_manager, err := services.GetJITManager(org_config_obj)
			if err != nil {
				returnError(w, 500, "JIT service not available")
				return
			}

			result, err := jit_manager.ApproveOrDeny(
				org_config_obj, user_record.Name, approval)
			if err != nil {
				returnError(w, 400, err.Error())
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})
}

func jitRevokeHandler() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				returnError(w, 405, "Method not allowed")
				return
			}

			org_id := authenticators.GetOrgIdFromRequest(r)
			org_id = utils.NormalizedOrgId(org_id)

			org_manager, err := services.GetOrgManager()
			if err != nil {
				returnError(w, 500, err.Error())
				return
			}

			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			user_record := GetUserInfo(r.Context(), org_config_obj)
			if user_record.Name == "" {
				returnError(w, 403, "Unauthenticated")
				return
			}

			body, ok := readJITBody(w, r)
			if !ok {
				return
			}

			var req struct {
				RequestId string `json:"request_id"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				returnError(w, 400, "Invalid request body")
				return
			}

			jit_manager, err := services.GetJITManager(org_config_obj)
			if err != nil {
				returnError(w, 500, "JIT service not available")
				return
			}

			err = jit_manager.RevokeGrant(
				org_config_obj, user_record.Name, req.RequestId)
			if err != nil {
				returnError(w, 400, err.Error())
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{}`))
		})
}

func jitListHandler() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			org_id := authenticators.GetOrgIdFromRequest(r)
			org_id = utils.NormalizedOrgId(org_id)

			org_manager, err := services.GetOrgManager()
			if err != nil {
				returnError(w, 500, err.Error())
				return
			}

			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			user_record := GetUserInfo(r.Context(), org_config_obj)
			if user_record.Name == "" {
				returnError(w, 403, "Unauthenticated")
				return
			}

			jit_manager, err := services.GetJITManager(org_config_obj)
			if err != nil {
				returnError(w, 500, "JIT service not available")
				return
			}

			// Non-admins can only see their own requests
			is_admin, _ := services.CheckAccess(
				org_config_obj, user_record.Name, acls.SERVER_ADMIN)

			status_str := r.URL.Query().Get("status")
			username := r.URL.Query().Get("username")

			if !is_admin {
				// Force non-admins to only see their own requests
				username = user_record.Name
			}

			var status api_proto.JITRequestStatus = -1
			switch status_str {
			case "PENDING":
				status = api_proto.JIT_STATUS_PENDING
			case "APPROVED":
				status = api_proto.JIT_STATUS_APPROVED
			case "DENIED":
				status = api_proto.JIT_STATUS_DENIED
			case "EXPIRED":
				status = api_proto.JIT_STATUS_EXPIRED
			case "REVOKED":
				status = api_proto.JIT_STATUS_REVOKED
			}

			result, err := jit_manager.ListRequests(
				org_config_obj, status, username)
			if err != nil {
				returnError(w, 500, err.Error())
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})
}

func jitMyGrantsHandler() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			org_id := authenticators.GetOrgIdFromRequest(r)
			org_id = utils.NormalizedOrgId(org_id)

			org_manager, err := services.GetOrgManager()
			if err != nil {
				returnError(w, 500, err.Error())
				return
			}

			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			user_record := GetUserInfo(r.Context(), org_config_obj)
			if user_record.Name == "" {
				returnError(w, 403, "Unauthenticated")
				return
			}

			jit_manager, err := services.GetJITManager(org_config_obj)
			if err != nil {
				returnError(w, 500, "JIT service not available")
				return
			}

			grants, err := jit_manager.GetActiveGrants(
				org_config_obj, user_record.Name)
			if err != nil {
				returnError(w, 500, err.Error())
				return
			}

			result := &api_proto.JITRoleRequests{Items: grants}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})
}
