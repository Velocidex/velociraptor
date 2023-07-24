package glob

import (
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/protocols"
)

// Define a protocol for glob objects so we can hide deprecated fields
// and expose additional fields.
type _GlobHitAssociativeProtocol struct{}

func (self _GlobHitAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {

	_, ok := a.(*GlobHit)
	if !ok {
		return false
	}

	switch b.(type) {
	case string:
		break
	default:
		return false
	}

	return true

}

func (self _GlobHitAssociativeProtocol) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {

	// Only expose some fields that are explicitly provided by the
	// glob.FileInfo interface. This cleans up * expansion in SELECT *
	// FROM ...
	return []string{
		"Name", "OSPath", "Mtime", "Atime", "Ctime",
		"Btime", "Size", "Mode", "IsDir", "IsLink", "Data", "Globs",
	}
}

func (self _GlobHitAssociativeProtocol) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {

	// Issue a deprecation warning if needed but only issue it once per query.
	b_str, ok := b.(string)
	if ok && b_str == "FullPath" {
		scope.Log("Deprecation: The FullPath column of the Glob plugin is deprecated and will be removed soon - Use OSPath instead")
	}

	return protocols.DefaultAssociative{}.Associative(scope, a, b)
}

func init() {
	vql_subsystem.RegisterProtocol(&_GlobHitAssociativeProtocol{})
}

func getUniqueName(f accessors.FileInfo) string {
	uniquer, ok := f.(accessors.UniqueBasename)
	if ok {
		return uniquer.UniqueName()
	}
	return f.Name()
}
