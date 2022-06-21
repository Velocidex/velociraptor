package main

import (
	"fmt"
	"os"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
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

func Prelog(format string, v ...interface{}) {
	// If the logging_flag is specified we log prelogs immediately -
	// this is necessary to be able to capture problems with the
	// config file because we are unable to initialize our real
	// logging system until we have a valid config.
	if *logging_flag != "" {
		fd, err := os.OpenFile(*logging_flag, os.O_RDWR|os.O_CREATE, 0600)
		if err == nil {
			fd.Seek(0, os.SEEK_END)

			// Write a JSONL log line
			fd.Write([]byte(json.Format(
				`{"level":"prelog","msg":%q,"time":%q}`,
				fmt.Sprintf(format, v...), time.Now(),
			)))
			fd.Write([]byte("\n"))
			fd.Close()
			return
		}
	}

	logging.Prelog(format, v...)
}
