package server_monitoring

import (
	"context"
	"encoding/json"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type serverLogger struct {
	config_obj   *config_proto.Config
	path_manager api.PathManager
	artifact     string
	ctx          context.Context
}

func (self *serverLogger) Write(b []byte) (int, error) {
	level, msg := logging.SplitIntoLevelAndLog(b)

	writer, err := timed.NewTimedResultSetWriter(
		self.config_obj, self.path_manager, nil,
		utils.BackgroundWriter)
	if err != nil {
		return 0, err
	}
	defer writer.Close()

	// Logs for event queries are written to timed result sets just
	// like the regular artifacts.
	msg = artifacts.DeobfuscateString(self.config_obj, msg)
	writer.Write(ordereddict.NewDict().
		Set("Timestamp", utils.GetTime().Now().UTC().String()).
		Set("Level", level).
		Set("Message", msg))

	if level == logging.ALERT {
		err := self.processAlert(msg)
		if err != nil {
			return 0, err
		}
	}

	return len(b), nil
}

func (self *serverLogger) processAlert(msg string) error {
	alert := &services.AlertMessage{}
	err := json.Unmarshal([]byte(msg), alert)
	if err != nil {
		return err
	}

	alert.ClientId = "server"
	alert.Artifact = self.artifact
	alert.ArtifactType = "SERVER_MONITORING"

	serialized, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	serialized = append(serialized, '\n')

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}
	return journal.PushJsonlToArtifact(self.ctx, self.config_obj,
		serialized, 1, "Server.Internal.Alerts", "server", "")
}
