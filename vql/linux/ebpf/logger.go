//go:build linux && (arm64 || amd64)
// +build linux
// +build arm64 amd64

package ebpf

import (
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/vfilter"
)

type Logger struct {
	mu         sync.Mutex
	config_obj *config_proto.Config
	scope      vfilter.Scope
}

func NewLogger(config_obj *config_proto.Config) *Logger {
	return &Logger{
		config_obj: config_obj,
	}
}

func (self *Logger) Log(format string, a ...interface{}) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.scope != nil {
		self.scope.Log(format, a...)
	} else {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Info(format, a...)
	}
}

func (self *Logger) SetScope(scope vfilter.Scope) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.scope = scope
	_ = scope.AddDestructor(func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		self.scope = nil
	})
}

func (self *Logger) Error(format string, a ...interface{}) {
	self.Log("Error: "+format, a...)
}

func (self *Logger) Warn(format string, a ...interface{}) {
	self.Log("Warn: "+format, a...)
}

func (self *Logger) Debug(format string, a ...interface{}) {
	self.Log("Debug: "+format, a...)
}
