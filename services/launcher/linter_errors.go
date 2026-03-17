package launcher

import (
	"regexp"

	"www.velocidex.com/golang/velociraptor/utils"
)

// Possible linter errors and their names.
const (
	UNKNOWN_PARAMETER_IN_CALL     = "unknown_parameter_in_call"
	UNKNOWN_PARAMETER_IN_CALL_MSG = "Call to %[1]v contains unknown parameter %[2]v"

	UNKNOWN_ARTIFACT_IN_QUERY     = "unknown_artifact_in_query"
	UNKNOWN_ARTIFACT_IN_QUERY_MSG = "Query calls Unknown artifact %[1]v"

	UNKNOWN_PLUGIN     = "unknown_plugin"
	UNKNOWN_PLUGIN_MSG = "Unknown %[2]v %[1]v()"

	KWARGS_MIXED_CALL     = "kwargs_mixed_call"
	KWARGS_MIXED_CALL_MSG = "Calling %[3]v `%[1]v` with kwargs (**) as well as with arg `%[2]v`"

	CALL_AS_FUNCTION                    = "call_as_function"
	CALL_AS_FUNCTION_MSG                = "LET %[1]v was defined without args but it is being called as a function"
	INVALID_ARG                         = "invalid_arg"
	INVALID_ARG_FOR_DEFINITION_MSG      = "Invalid arg %[1]v for VQL definition %[2]v"
	INVALID_ARG_FOR_PLUGIN_MSG          = "Invalid arg %[1]v for %[2]v %[3]v()"
	REQUIRED_ARG_MISSING                = "required_arg_missing"
	REQUIRED_ARG_MISSING_MSG            = "While calling VQL definition %[2]v(), required arg %[1]v is not provided"
	REQUIRED_ARG_MISSING_FOR_PLUGIN_MSG = "While calling %[2]v %[3]v(), required arg %[1]v is not provided"

	INVALID_IMPORT               = "invalid_import"
	INVALID_IMPORT_MSG           = "%[2]v: Artifact %[1]v not found"
	INVALID_IMPORT_NO_EXPORT_MSG = "%[2]v: Artifact %[1]v does not export anything"

	ARTIFACT_VQL_ERROR       = "artifact_vql_error"
	ARTIFACT_VQL_PRECOND_MSG = "%[1]v: precondition: %[2]v"
	ARTIFACT_VQL_EXPORT_MSG  = "%[1]v: export: %[2]v"
	ARTIFACT_VQL_QUERY_MSG   = "%[1]v: query: %[2]v"

	YAML_ERROR     = "yaml_error"
	YAML_ERROR_MSG = "YAML Error: %[1]v"

	// Warnings
	REQUIRED_PERMISSIONS     = "required_permissions"
	REQUIRED_PERMISSIONS_MSG = "Add %[1]v to artifact's required_permissions or implied_permissions fields"

	SYMBOL_MASK_WARN     = "symbol_mask_warn"
	SYMBOL_MASK_WARN_MSG = "Use of symbol `%[1]v` which might mask a %[2]v of the same name"

	INVALID_SUPPRESSION             = "invalid_suppression"
	INVALID_SUPPRESSION_MSG         = "Suppression %[1]v not valid: %[2]v"
	INVALID_SUPPRESSION_UNKNOWN_MSG = "Suppression %[1]v not known"
)

func validLinterDirectives(name string) bool {
	switch name {
	case UNKNOWN_PARAMETER_IN_CALL, UNKNOWN_ARTIFACT_IN_QUERY,
		UNKNOWN_PLUGIN, KWARGS_MIXED_CALL, CALL_AS_FUNCTION, INVALID_ARG,
		REQUIRED_ARG_MISSING, INVALID_IMPORT, ARTIFACT_VQL_ERROR, YAML_ERROR,
		REQUIRED_PERMISSIONS, SYMBOL_MASK_WARN, INVALID_SUPPRESSION:
		return true
	}
	return false
}

var (
	// A linter directive in the comment to add a linter suppression
	suppressionRegex = regexp.MustCompile(`(?i)linter: *([a-z_]+):("[^"]+"|'[^']+'|[^ ]+|)`)
)

func (self *AnalysisState) ParseSuppressions(comments []string) {
	for _, c := range comments {
		for _, m := range suppressionRegex.FindAllStringSubmatch(c, -1) {

			name := m[1]
			if !validLinterDirectives(name) {
				self.SetError(INVALID_SUPPRESSION,
					INVALID_SUPPRESSION_UNKNOWN_MSG, name)
				continue
			}

			expression := m[2]
			if len(expression) >= 2 &&
				expression[0] == '"' &&
				expression[len(expression)-1] == '"' {
				expression = expression[1 : len(expression)-1]
			}

			// Try to compile the expression as a regex
			re, err := regexp.Compile(expression)
			if err != nil {
				self.SetError(INVALID_SUPPRESSION,
					INVALID_SUPPRESSION_MSG, name, err)
				continue
			}

			self.Suppressions = append(self.Suppressions, Suppression{
				Name:         name,
				Subject:      expression,
				subjectRegex: re,
			})
		}
	}
}

// Does this error match a suppression?
func (self *AnalysisState) matchSuppression(name string, args ...interface{}) bool {
	if len(args) == 0 {
		return false
	}
	for _, s := range self.Suppressions {
		if s.Name == name {
			subject := utils.ToString(args[0])
			if s.subjectRegex.MatchString(subject) {
				return true
			}
		}
	}

	return false
}
