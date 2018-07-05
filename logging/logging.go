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
		config: config,
	}

	return &result
}

func (self *Logger) Error(format string, v ...interface{}) {
	if self.error_log == nil {
		self.error_log = log.New(os.Stderr, "ERR:", log.LstdFlags)
	}

	self.error_log.Printf(format, v...)
}

func (self *Logger) Info(format string, v ...interface{}) {
	if self.info_log == nil {
		self.info_log = log.New(os.Stderr, "INFO:", log.LstdFlags)
	}
	self.info_log.Printf(format, v...)
}
