// Manage reading and writing result sets.

// Velociraptor is essentially a VQL engine - all operations are
// simply queries and all queries return a result set. Result sets are
// essentially tables - containing columns specified by the query
// itself and rows.

// Usually queries are encapsulated within artifacts so they contain a
// name and a type. Velociraptor writes these result sets to various
// places in the file store based on the artifact type and its name.

// This module abstracts the specific location by simply providing an
// interface for code to read and write various artifact's result
// sets.

package result_sets

import (
	"fmt"

	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func GetArtifactMode(config_obj *config_proto.Config, artifact_name string) (int, error) {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return 0, err
	}

	repository, _ := manager.GetGlobalRepository(config_obj)

	artifact, pres := repository.Get(config_obj, artifact_name)
	if !pres {
		return 0, fmt.Errorf("Artifact %s not known", artifact_name)
	}

	return paths.ModeNameToMode(artifact.Type), nil
}

type ResultSetWriter struct {
	rows []*ordereddict.Dict
	opts *json.EncOpts
	fd   api.FileWriter
}

func (self *ResultSetWriter) WriteJSONL(serialized []byte) {
	_, _ = self.fd.Write(serialized)
}

func (self *ResultSetWriter) Write(row *ordereddict.Dict) {
	self.rows = append(self.rows, row)
	if len(self.rows) > 10000 {
		self.Flush()
	}
}

func (self *ResultSetWriter) Flush() {
	serialized, err := utils.DictsToJson(self.rows, self.opts)
	if err == nil {
		_, _ = self.fd.Write(serialized)
	}
	self.rows = nil
}

func (self *ResultSetWriter) Close() {
	self.Flush()
	self.fd.Close()
}

func NewResultSetWriter(
	config_obj *config_proto.Config,
	path_manager api.PathManager,
	opts *json.EncOpts,
	truncate bool) (*ResultSetWriter, error) {
	file_store_factory := file_store.GetFileStore(config_obj)
	log_path, err := path_manager.GetPathForWriting()
	if err != nil {
		return nil, err
	}

	fd, err := file_store_factory.WriteFile(log_path)
	if err != nil {
		return nil, err
	}

	if truncate {
		err = fd.Truncate()
		if err != nil {
			fd.Close()
			return nil, err
		}
	}

	return &ResultSetWriter{fd: fd, opts: opts}, nil
}
