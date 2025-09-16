package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"

	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

func toolUploadHandler() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			org_id := authenticators.GetOrgIdFromRequest(r)
			org_manager, err := services.GetOrgManager()
			if err != nil {
				returnError(w, http.StatusUnauthorized, err.Error())
				return
			}

			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			if err != nil {
				returnError(w, http.StatusUnauthorized, err.Error())
				return
			}

			// Check for acls
			userinfo := GetUserInfo(r.Context(), org_config_obj)
			permissions := acls.ARTIFACT_WRITER
			perm, err := services.CheckAccess(org_config_obj, userinfo.Name, permissions)
			if !perm || err != nil {
				returnError(w, http.StatusUnauthorized,
					"User is not allowed to upload tools.")
				return
			}

			// Parse our multipart form, 10 << 20 specifies a maximum
			// upload of 10 MB files.
			err = r.ParseMultipartForm(10 << 25)
			if err != nil {
				returnError(w, http.StatusBadRequest, "Unsupported params")
				return
			}
			defer func() {
				err := r.MultipartForm.RemoveAll()
				if err != nil {
					logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
					logger.Error("toolUploadHandler MultipartForm.RemoveAll: %v", err)
				}
			}()

			tool := &artifacts_proto.Tool{}
			params, pres := r.Form["_params_"]
			if !pres || len(params) != 1 {
				returnError(w, http.StatusBadRequest, "Unsupported params")
				return
			}

			err = json.Unmarshal([]byte(params[0]), tool)
			if err != nil {
				returnError(w, http.StatusBadRequest, "Unsupported params")
				return
			}

			// FormFile returns the first file for the given key `myFile`
			// it also returns the FileHeader so we can get the Filename,
			// the Header and the size of the file
			file, handler, err := r.FormFile("file")
			if err != nil {
				returnError(w, 403, fmt.Sprintf("Unsupported params: %v", err))
				return
			}
			defer file.Close()

			tool.Filename = path.Base(handler.Filename)
			tool.ServeLocally = true

			path_manager := paths.NewInventoryPathManager(org_config_obj, tool)
			pathspec, file_store_factory, err := path_manager.Path()
			if err != nil {
				returnError(w, 404, err.Error())
			}

			writer, err := file_store_factory.WriteFile(pathspec)
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}
			defer writer.Close()

			err = writer.Truncate()
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}

			sha_sum := sha256.New()

			_, err = io.Copy(writer, io.TeeReader(file, sha_sum))
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}

			tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))

			inventory, err := services.GetInventory(org_config_obj)
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}

			ctx := r.Context()
			err = inventory.AddTool(ctx, org_config_obj, tool,
				services.ToolOptions{
					AdminOverride: true,
				})
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}

			// Now materialize the tool
			tool, err = inventory.GetToolInfo(
				r.Context(), org_config_obj, tool.Name, tool.Version)
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}

			serialized, _ := json.Marshal(tool)
			_, err = w.Write(serialized)
			if err != nil {
				logger := logging.GetLogger(org_config_obj, &logging.GUIComponent)
				logger.Error("toolUploadHandler: %v", err)
			}
		})
}

func formUploadHandler() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			org_id := authenticators.GetOrgIdFromRequest(r)
			org_manager, err := services.GetOrgManager()
			if err != nil {
				returnError(w, http.StatusUnauthorized, err.Error())
				return
			}

			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			if err != nil {
				returnError(w, http.StatusUnauthorized, err.Error())
				return
			}

			// Check for acls
			userinfo := GetUserInfo(r.Context(), org_config_obj)
			permissions := acls.COLLECT_CLIENT
			perm, err := services.CheckAccess(org_config_obj, userinfo.Name, permissions)
			if !perm || err != nil {
				returnError(w, http.StatusUnauthorized,
					"User is not allowed to upload files for forms.")
				return
			}

			// Parse our multipart form, 10 << 20 specifies a maximum
			// upload of 10 MB files.
			err = r.ParseMultipartForm(10 << 20)
			if err != nil {
				returnError(w, http.StatusBadRequest, "Unsupported params")
				return
			}
			defer func() {
				err := r.MultipartForm.RemoveAll()
				if err != nil {
					logger := logging.GetLogger(org_config_obj, &logging.GUIComponent)
					logger.Error("formUploadHandler MultipartForm.RemoveAll: %v", err)
				}
			}()

			form_desc := &api_proto.FormUploadMetadata{}
			params, pres := r.Form["_params_"]
			if !pres || len(params) != 1 {
				returnError(w, http.StatusBadRequest, "Unsupported params")
				return
			}

			err = json.Unmarshal([]byte(params[0]), form_desc)
			if err != nil {
				returnError(w, http.StatusBadRequest, "Unsupported params")
				return
			}

			// FormFile returns the first file for the given key `file`
			// it also returns the FileHeader so we can get the Filename,
			// the Header and the size of the file
			file, handler, err := r.FormFile("file")
			if err != nil {
				returnError(w, 403, fmt.Sprintf("Unsupported params: %v", err))
				return
			}
			defer file.Close()

			form_desc.Filename = path.Base(handler.Filename)

			path_manager := paths.NewFormUploadPathManager(
				org_config_obj, form_desc.Filename)

			pathspec, file_store_factory, err := path_manager.Path()
			if err != nil {
				returnError(w, 403, fmt.Sprintf("Error: %v", err))
				return
			}

			form_desc.Url = path_manager.URL()
			form_desc.VfsPath = pathspec.Components()

			writer, err := file_store_factory.WriteFile(pathspec)
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}
			defer writer.Close()

			err = writer.Truncate()
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}

			_, err = io.Copy(writer, file)
			if err != nil {
				returnError(w, http.StatusInternalServerError,
					fmt.Sprintf("Error: %v", err))
				return
			}

			serialized, _ := json.Marshal(form_desc)
			_, err = w.Write(serialized)
			if err != nil {
				logger := logging.GetLogger(org_config_obj, &logging.GUIComponent)
				logger.Error("toolUploadHandler: %v", err)
			}
		})
}
