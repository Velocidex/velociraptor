package main

import (
	"fmt"
	"os"
	"time"

	errors "github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

type LogWriter struct {
	config_obj *config_proto.Config
	Error      error
}

func (self *LogWriter) Write(b []byte) (int, error) {
	level, msg := logging.SplitIntoLevelAndLog(b)
	if level == "ERROR" {
		self.Error = errors.New(msg)
	}
	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.LogWithLevel(level, "%s", msg)

	return len(b), nil
}

type StdoutLogWriter struct {
	Error error
}

func (self *StdoutLogWriter) Write(b []byte) (int, error) {
	level, msg := logging.SplitIntoLevelAndLog(b)
	if level == "ERROR" {
		self.Error = errors.New(msg)
	}

	fmt.Printf("%v", string(b))
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
			_, err := fd.Seek(0, os.SEEK_END)
			if err == nil {
				// Write a JSONL log line
				_, _ = fd.Write([]byte(json.Format(
					`{"level":"prelog","msg":%q,"time":%q}`,
					fmt.Sprintf(format, v...), time.Now(),
				)))
				_, _ = fd.Write([]byte("\n"))
				fd.Close()
			}
			return
		}
	}

	logging.Prelog(format, v...)
}
