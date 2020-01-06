package main

import (
	"log"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/vfilter"
)

type LogWriter struct {
	config_obj *config_proto.Config
}

func (self *LogWriter) Write(b []byte) (int, error) {
	logging.GetLogger(self.config_obj, &logging.ClientComponent).Info("%v", string(b))
	return len(b), nil
}

func AddLogger(scope *vfilter.Scope,
	config_obj *config_proto.Config) {
	scope.Logger = log.New(&LogWriter{config_obj}, "Velociraptor: ", log.Lshortfile)
}
