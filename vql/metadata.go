package vql

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
)

// Where is the plugin allowed to run on.
type ExecutionContext string

var (
	// Plugin is only allowed to run on the master node:
	// - Clients will reject the plugin.
	// - Minions will forward calls to the master node.
	MasterExecutionContext ExecutionContext = "master"
)

type MetadataBuilder struct {
	*ordereddict.Dict
}

func (self *MetadataBuilder) Permissions(perms ...acls.ACL_PERMISSION) *MetadataBuilder {
	parts := []string{}
	for _, p := range perms {
		parts = append(parts, p.String())
	}
	self.Set("permissions", strings.Join(parts, ","))
	return self
}

func (self *MetadataBuilder) Build() *ordereddict.Dict {
	// Report the location of our caller - this is usually the Info()
	// function of the plugin definition.
	pc, _, _, ok := runtime.Caller(1)
	if ok {
		fn := runtime.FuncForPC(pc)
		file, line := fn.FileLine(pc)
		self.Dict.
			Set("Line", line).
			Set("File", strings.TrimPrefix(file, ProjectRoot)).
			Set("Fn", fn.Name())
	}
	return self.Dict
}

// Marks the plugins as an event plugin
func (self *MetadataBuilder) Event() *MetadataBuilder {
	self.Set("event", true)
	return self
}

// Used to tag plugins or functions that only run on the master node.
func (self *MetadataBuilder) ExecutionContext(
	ctx ...ExecutionContext) *MetadataBuilder {
	parts := []string{}
	for _, p := range ctx {
		parts = append(parts, string(p))
	}
	self.Set("execution", strings.Join(parts, ","))
	return self
}

func VQLMetadata() *MetadataBuilder {
	return &MetadataBuilder{ordereddict.NewDict()}
}

var (
	ProjectRoot = projectRoot()
)

func projectRoot() string {
	_, file, _, _ := runtime.Caller(0)
	res := filepath.Dir(filepath.Dir(file)) + string(filepath.Separator)
	res = strings.ReplaceAll(res, "\\", "/")
	return res
}
