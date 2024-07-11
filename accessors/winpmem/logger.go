package winpmem

import (
	"www.velocidex.com/golang/vfilter/types"
)

type ScopeLogger struct {
	scope  types.Scope
	prefix string
	debug  bool
}

func (self *ScopeLogger) Info(format string, args ...interface{}) {
	self.scope.Log("INFO:"+self.prefix+format, args...)
}

func (self *ScopeLogger) Debug(format string, args ...interface{}) {
	if self.debug {
		self.scope.Debug("DEBUG:"+self.prefix+format, args...)
	}
}

func (self *ScopeLogger) SetDebug() {
	self.debug = true
}

func (self *ScopeLogger) Progress(pages int)    {}
func (self *ScopeLogger) SetProgress(pages int) {}

func NewLogger(scope types.Scope, prefix string) *ScopeLogger {
	return &ScopeLogger{scope: scope, prefix: prefix}
}
