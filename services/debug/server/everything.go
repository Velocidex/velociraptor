package server

import (
	"archive/zip"
	"encoding/json"
	"net/http"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

func (self *debugMux) handleEverything(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; profiles.zip")
	w.WriteHeader(200)

	zip_writer := zip.NewWriter(w)
	defer zip_writer.Close()

	builder := services.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	seen := make(map[string]bool)

	for _, i := range debug.GetProfileWriters() {
		_, pres := seen[i.Name]
		if pres {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("Non unique profile name %v", i.Name)
		}
		seen[i.Name] = true

		fd, err := zip_writer.CreateHeader(&zip.FileHeader{
			Name:   i.Name,
			Method: zip.Deflate,
		})
		if err != nil {
			continue
		}

		output_chan := make(chan vfilter.Row)
		go func() {
			defer close(output_chan)

			i.ProfileWriter(r.Context(), scope, output_chan)
		}()

		for row := range output_chan {
			serialized, _ := json.Marshal(row)
			serialized = append(serialized, '\n')
			_, _ = fd.Write(serialized)
		}
	}
}
