package logging

import (
	"log"
	"os"
	"sync"

	"github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

type stackTracer interface {
	StackTrace() errors.StackTrace
}

type Logger struct {
	mu        sync.Mutex
	config    *api_proto.Config
	error_log *log.Logger
	info_log  *log.Logger
}

type logWriter struct {
	logger *Logger
}

func (self *logWriter) Write(b []byte) (int, error) {
	self.logger.Info("%s", string(b))
	return len(b), nil
}

// A log compatible logger.
func NewPlainLogger(config *api_proto.Config) *log.Logger {
	return log.New(&logWriter{NewLogger(config)}, "", log.Lshortfile)
}

func NewLogger(config *api_proto.Config) *Logger {
	result := Logger{
		config: config,
	}

	return &result
}

func (self *Logger) _Error(format string, v ...interface{}) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.error_log == nil {
		self.error_log = log.New(os.Stderr, "ERR:", log.LstdFlags)
	}

	self.error_log.Printf(format, v...)
}

func (self *Logger) Error(msg string, err error) {
	s_err, ok := err.(stackTracer)
	if ok {
		st := s_err.StackTrace()
		self._Error("ERR: %s %s %+v", msg, err.Error(), st)
	} else {
		self._Error("ERR: %s %s", msg, err.Error())
	}
}

func (self *Logger) Info(format string, v ...interface{}) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.info_log == nil {
		self.info_log = log.New(os.Stderr, "INFO:", log.LstdFlags)
	}
	self.info_log.Printf(format, v...)
}
