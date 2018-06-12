package logging

import (
	"log"
	"os"
	"www.velocidex.com/golang/velociraptor/config"
)

type Logger struct {
	config    *config.Config
	error_log *log.Logger
	info_log  *log.Logger
}

func NewLogger(config *config.Config) *Logger {
	result := Logger{
		config:    config,
		error_log: log.New(os.Stderr, "ERR:", log.LstdFlags),
		info_log:  log.New(os.Stderr, "INFO:", log.LstdFlags),
	}

	return &result
}

func (self *Logger) Error(format string, v ...interface{}) {
	self.error_log.Printf(format, v...)
}

func (self *Logger) Info(format string, v ...interface{}) {
	self.info_log.Printf(format, v...)
}
