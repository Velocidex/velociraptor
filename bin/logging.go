package main

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

type LogWriter struct {
	config_obj *config_proto.Config
}

func (self *LogWriter) Write(b []byte) (int, error) {
	level, msg := logging.SplitIntoLevelAndLog(b)
	logging.GetLogger(self.config_obj, &logging.ClientComponent).
		LogWithLevel(level, "%s", msg)

	return len(b), nil
}
