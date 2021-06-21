package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"

	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

func toolUploadHandler(
	config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for acls
		userinfo := GetUserInfo(r.Context(), config_obj)
		permissions := acls.ARTIFACT_WRITER
		perm, err := acls.CheckAccess(config_obj, userinfo.Name, permissions)
		if !perm || err != nil {
			returnError(w, http.StatusUnauthorized,
				"User is not allowed to upload tools.")
			return
		}

		// Parse our multipart form, 10 << 20 specifies a maximum
		// upload of 10 MB files.
		err = r.ParseMultipartForm(10 << 20)
		if err != nil {
			returnError(w, http.StatusBadRequest, "Unsupported params")
			return
		}
		defer r.MultipartForm.RemoveAll()

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

		file_store_factory := file_store.GetFileStore(config_obj)
		path_manager := paths.NewInventoryPathManager(config_obj, tool)
		writer, err := file_store_factory.WriteFile(path_manager.Path())
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

		err = services.GetInventory().AddTool(config_obj, tool,
			services.ToolOptions{
				AdminOverride: true,
			})
		if err != nil {
			returnError(w, http.StatusInternalServerError,
				fmt.Sprintf("Error: %v", err))
			return
		}

		// Now materialize the tool
		tool, err = services.GetInventory().GetToolInfo(
			r.Context(), config_obj, tool.Name)
		if err != nil {
			returnError(w, http.StatusInternalServerError,
				fmt.Sprintf("Error: %v", err))
			return
		}

		serialized, _ := json.Marshal(tool)
		_, err = w.Write(serialized)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Error("toolUploadHandler: %v", err)
		}
	})
}
