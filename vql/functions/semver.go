/*
Function to support the parsing and comparison of semantic version strings.

This function parses a given semantic version string and extracts the major,
minor, and patch versions. It also allows semantic versions to be compared
against regular version strings. It supports greater than, less than and
equal to comparisons.

For example, the following expressions evaluate to true:

  - semver(version="1.0.0") = "1.0.0"
  - semver(version="1.0.0") > "0.5.0"
  - semver(version="1.0.0") < "2.0.0"
*/
package functions

import (
	"context"
	"errors"

	semver "github.com/Masterminds/semver/v3"
	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var ErrBadSemver = errors.New("Could not parse semantic version!")

// SemverResult represents the result of parsing a semantic version.
type SemverResult struct {
	parsed     *semver.Version
	Major      uint64
	Minor      uint64
	Patch      uint64
	Prerelease string
	Version    string
}

// NewSemverResult creates a new SemverResult instance from a version string.
func NewSemverResult(version string) (*SemverResult, error) {
	result := &SemverResult{}

	parsed, err := semver.NewVersion(version)
	if err != nil {
		return result, err
	}

	result.parsed = parsed
	result.Major = parsed.Major()
	result.Minor = parsed.Minor()
	result.Patch = parsed.Patch()
	result.Prerelease = parsed.Prerelease()
	result.Version = parsed.String()

	return result, nil
}

// IsSemverResult returns whether the given value is a SemverResult type.
func IsSemverResult(value vfilter.Any) (*SemverResult, bool) {
	switch v := value.(type) {
	case *SemverResult:
		return v, true
	case SemverResult:
		return &v, true
	default:
		return &SemverResult{}, false
	}
}

/*
SemverFromAny attempts to convert any given value to a SemverResult.

It currently only supports the conversion of strings to SemverResults
for comparison.
*/
func SemverFromAny(ctx context.Context, scope vfilter.Scope, value vfilter.Any) (*SemverResult, error) {
	switch v := value.(type) {
	case vfilter.LazyExpr:
		return SemverFromAny(ctx, scope, v.ReduceWithScope(ctx, scope))

	case string:
		result, err := NewSemverResult(v)
		return result, err

	case SemverResult:
		return &v, nil

	case *SemverResult:
		return v, nil

	case nil, types.Null, *types.Null:
		return &SemverResult{}, ErrBadSemver

	default:
		str, ok := v.(string)
		if !ok {
			return &SemverResult{}, ErrBadSemver
		}

		result, err := NewSemverResult(str)
		return result, err
	}
}

// GreaterThan returns whether this version is greater than the other.
func (self *SemverResult) GreaterThan(other *SemverResult) bool {
	return self.parsed.GreaterThan(other.parsed)
}

// LessThan returns whether this version is less than the other.
func (self *SemverResult) LessThan(other *SemverResult) bool {
	return self.parsed.LessThan(other.parsed)
}

// Equals returns whether this version is equal to the other.
func (self *SemverResult) Equals(other *SemverResult) bool {
	return self.parsed.Equal(other.parsed)
}

type SemverArgs struct {
	Version string `vfilter:"required,field=version,doc=A string to convert to a semantic version"`
}

type SemverFunction struct{}

func (self *SemverFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "semver", args)()

	arg := &SemverArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("semver: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Version == "" {
		return vfilter.Null{}
	}

	result, err := NewSemverResult(arg.Version)
	if err != nil {
		if errors.Is(err, semver.ErrInvalidSemVer) {
			scope.Log("semver: invalid semantic version '%s'", arg.Version)
		} else {
			scope.Log("semver: %s", err.Error())
		}

		return vfilter.Null{}
	}

	return result
}

func (self SemverFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "semver",
		Doc:     "Parse a semantic version string.",
		ArgType: type_map.AddType(scope, &SemverArgs{}),
		Version: 1,
	}
}

type _SemverLtString struct{}

func (self _SemverLtString) getVersions(ctx context.Context, scope vfilter.Scope,
	a, b vfilter.Any) (*SemverResult, *SemverResult, bool) {
	a_ver, a_is_ver := IsSemverResult(a)
	b_ver, b_is_ver := IsSemverResult(b)
	a_str, a_is_str := a.(string)
	b_str, b_is_str := b.(string)

	if a_is_ver && b_is_ver {
		return a_ver, b_ver, true
	}

	if a_is_ver && b_is_str {
		b_ver, err := SemverFromAny(ctx, scope, b_str)
		if err != nil {
			return a_ver, b_ver, false
		}

		return a_ver, b_ver, true
	}

	if a_is_str && b_is_ver {
		a_ver, err := SemverFromAny(ctx, scope, a_str)
		if err != nil {
			return a_ver, b_ver, false
		}

		return a_ver, b_ver, true
	}

	return &SemverResult{}, &SemverResult{}, false
}

func (self _SemverLtString) Lt(scope vfilter.Scope, a, b vfilter.Any) bool {
	a_ver, b_ver, ok := self.getVersions(context.Background(), scope, a, b)
	if !ok {
		return false
	}

	return a_ver.LessThan(b_ver)
}

func (self _SemverLtString) Applicable(a, b vfilter.Any) bool {
	_, a_is_ver := IsSemverResult(a)
	_, a_is_str := a.(string)
	_, b_is_ver := IsSemverResult(b)
	_, b_is_str := b.(string)

	if a_is_ver && b_is_ver {
		return true
	}

	if a_is_ver && b_is_str {
		return true
	}

	if a_is_str && b_is_ver {
		return true
	}

	return false
}

type _SemverGtString struct{}

func (self _SemverGtString) Gt(scope vfilter.Scope, a, b vfilter.Any) bool {
	a_ver, b_ver, ok := _SemverLtString{}.getVersions(context.Background(), scope, a, b)
	if !ok {
		return false
	}

	return a_ver.GreaterThan(b_ver)
}

func (self _SemverGtString) Applicable(a, b vfilter.Any) bool {
	return _SemverLtString{}.Applicable(a, b)
}

type _SemverEqString struct{}

func (self _SemverEqString) Eq(scope vfilter.Scope, a, b vfilter.Any) bool {
	a_ver, b_ver, ok := _SemverLtString{}.getVersions(context.Background(), scope, a, b)
	if !ok {
		return false
	}

	return a_ver.Equals(b_ver)
}

func (self _SemverEqString) Applicable(a, b vfilter.Any) bool {
	return _SemverLtString{}.Applicable(a, b)
}

func init() {
	vql_subsystem.RegisterFunction(&SemverFunction{})
	vql_subsystem.RegisterProtocol(&_SemverLtString{})
	vql_subsystem.RegisterProtocol(&_SemverGtString{})
	vql_subsystem.RegisterProtocol(&_SemverEqString{})
}
