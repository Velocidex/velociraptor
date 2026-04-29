package launcher

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/paths/artifact_modes"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	api_description = &ApiDescription{}
	error_messages  = map[string]string{
		"E001": "",
	}
)

type Suppression struct {
	Name    string
	Subject string

	subjectRegex *regexp.Regexp
}

// Contains the result of the static analysis.
type AnalysisState struct {
	Artifact    string
	Permissions []string
	Errors      []string
	Warnings    []string

	// Keep track of existing definitions in LET queries.
	Definitions  map[string]vfilter.DefinitionSite
	Suppressions []Suppression
}

func (self *AnalysisState) SetError(
	name string, message string, args ...interface{}) {
	if self.matchSuppression(name, args...) {
		return
	}
	self.Errors = append(self.Errors, fmt.Sprintf(name+":"+message, args...))
}

func (self *AnalysisState) AnalyseCall(
	callsite vfilter.CallSite, desc CallDescriptor) {
	self.Permissions = utils.Sort(utils.DeduplicateStringSlice(
		append(self.Permissions, desc.Permissions...)))
}

func (self *AnalysisState) AnalyseArtifactRequiredPermissions(
	artifact *artifacts_proto.Artifact) {
	artifact_mode := artifact_modes.ModeNameToMode(artifact.Type)

	// Only client artifacts enforce required permissions
	if artifact_mode != artifact_modes.MODE_CLIENT {
		return
	}

	// When a user runs an artifact on a client they implicitly have
	// these permissions.
	implied_permissions := []string{
		"FILESYSTEM_READ", "MACHINE_STATE",

		// Used by http_client on the client side.
		"COLLECT_SERVER",
	}

	// They also receive permissins required by the artifact because
	// this will be enforced on the server.
	implied_permissions = append(implied_permissions,
		artifact.RequiredPermissions...)

	// The artifact writer can also declare which permission are
	// safely controlled.
	implied_permissions = append(implied_permissions,
		artifact.ImpliedPermissions...)

	// Now go over all the permissions used by the artifact an warn
	// about all permissions that are not required
	for _, perm := range self.Permissions {
		if !utils.InString(implied_permissions, perm) {
			emitWarning(REQUIRED_PERMISSIONS, self,
				REQUIRED_PERMISSIONS_MSG, perm)
		}
	}
}

func NewAnalysisState(artifact string) *AnalysisState {
	return &AnalysisState{
		Artifact:    artifact,
		Definitions: make(map[string]vfilter.DefinitionSite),
	}
}

type Required bool

type CallDescriptor struct {
	ArgsRequired map[string]Required
	Permissions  []string

	FreeFormArgs bool
}

func (self *CallDescriptor) SetPermissions(api *api_proto.Completion) {
	if api.Metadata != nil {
		perms, pres := api.Metadata["permissions"]
		if pres {
			self.Permissions = strings.Split(perms, ",")
		}
	}
}

func NewCallDescriptor(api *api_proto.Completion) CallDescriptor {
	res := &CallDescriptor{
		ArgsRequired: make(map[string]Required),
		FreeFormArgs: api.FreeFormArgs,
	}

	res.SetPermissions(api)
	return *res
}

type ApiDescription struct {
	functions map[string]CallDescriptor
	plugins   map[string]CallDescriptor
}

func (self *ApiDescription) init() error {
	// Initialize if needed
	if self.functions == nil || self.plugins == nil {
		self.functions = make(map[string]CallDescriptor)
		self.plugins = make(map[string]CallDescriptor)

		apis, err := utils.LoadApiDescription()
		if err != nil {
			return err
		}

		for _, api := range apis {
			if api.Type == "Function" {
				desc := NewCallDescriptor(api)

				// Ignore ** kwargs type of call.
				desc.ArgsRequired["**"] = Required(false)
				for _, arg := range api.Args {
					desc.ArgsRequired[arg.Name] = Required(arg.Required)
				}
				self.functions[api.Name] = desc
			}
			if api.Type == "Plugin" {
				desc := NewCallDescriptor(api)

				// Ignore ** kwargs type of call.
				desc.ArgsRequired["**"] = Required(false)
				for _, arg := range api.Args {
					desc.ArgsRequired[arg.Name] = Required(arg.Required)
				}
				self.plugins[api.Name] = desc
			}
		}
	}
	return nil
}

func (self *ApiDescription) verifyArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository, artifact_name string,
	callsite vfilter.CallSite,
	state *AnalysisState, res []error) []error {

	artifact, pres := repository.Get(ctx, config_obj, artifact_name)
	if !pres {
		return emitError(UNKNOWN_ARTIFACT_IN_QUERY, state, res,
			UNKNOWN_ARTIFACT_IN_QUERY_MSG, artifact_name)
	}

	parameters := make(map[string]bool)

	// Some implicit parameters that are always allowed
	parameters["source"] = true
	parameters["preconditions"] = true

	for _, p := range artifact.Parameters {
		parameters[p.Name] = true
	}

	for _, arg := range callsite.Args {
		// If the artifact is called with kwargs we really have no
		// idea and we can not verify it at all - So just give up.
		if arg == "**" {
			return self.checkKWArgs(state, callsite, res)
		}
		_, pres := parameters[arg]
		if !pres {
			res = emitError(UNKNOWN_PARAMETER_IN_CALL, state, res,
				UNKNOWN_PARAMETER_IN_CALL_MSG,
				callsite.Name, arg)
		}
	}

	return res
}

// When calling a plugin with a ** kwargs we can not really do any of
// the callsite checks - We have no idea if we are calling required or
// optional args. The only check we can do is to make sure that the
// caller does not mix ** with regular args.
func (self *ApiDescription) checkKWArgs(
	state *AnalysisState, callsite vfilter.CallSite, errors []error) []error {

	for _, a := range callsite.Args {
		if a == "**" {
			continue
		}

		errors = emitError(KWARGS_MIXED_CALL, state, errors,
			KWARGS_MIXED_CALL_MSG, callsite.Name, a, callsite.Type)
	}

	return errors
}

func (self *ApiDescription) verifySymbol(
	callsite vfilter.CallSite,
	state *AnalysisState,
	res []error) []error {

	// Ignore some well known plugins that can be accessed as a symbol
	switch callsite.Name {
	case "Artifact":
		return res
	}

	// Check if the symbol is masking a plugin, function or LET
	// definition
	symbol_type := "plugin"
	_, pres := self.plugins[callsite.Name]
	if !pres {
		symbol_type = "function"
		_, pres = self.functions[callsite.Name]
		if !pres {
			symbol_type = "LET definition"
			var def vfilter.DefinitionSite
			def, pres = state.Definitions[callsite.Name]
			// If the definition is not a function definition it is ok
			// to call it directly.
			if pres && def.Args == nil {
				pres = false
			}
		}
	}

	if pres {
		emitWarning(SYMBOL_MASK_WARN, state,
			SYMBOL_MASK_WARN_MSG, callsite.Name, symbol_type)
	}

	return res
}

func (self *ApiDescription) verifyLETCall(
	callsite vfilter.CallSite, state *AnalysisState, res []error) []error {
	// Handle LET definitions
	desc, pres := state.Definitions[callsite.Name]
	if !pres {
		return emitError(UNKNOWN_PLUGIN, state, res,
			UNKNOWN_PLUGIN_MSG, callsite.Name, callsite.Type)
	}

	if desc.Args == nil && callsite.Args != nil {
		res = emitError(CALL_AS_FUNCTION, state, res,
			CALL_AS_FUNCTION_MSG, callsite.Name)
	}

	for _, arg := range callsite.Args {
		if arg == "**" {
			res = self.checkKWArgs(state, callsite, res)
			break
		}

		// The callsite is calling some unknown
		// parameter.
		if !utils.InString(desc.Args, arg) {
			res = emitError(INVALID_ARG, state, res,
				INVALID_ARG_FOR_DEFINITION_MSG, arg, callsite.Name)
		}
	}

	// Now check if any of the required args are missing
	for _, arg := range desc.Args {
		// The arg has a default so the caller does not have
		// to specify it.
		if utils.InString(desc.Defaults, arg) {
			continue
		}

		// The definition parameter is missing from the
		// caller's args - this is required so we need to
		// error.
		if !utils.InString(callsite.Args, arg) {
			res = emitError(REQUIRED_ARG_MISSING, state, res,
				REQUIRED_ARG_MISSING_MSG, arg, callsite.Name)
		}
	}

	return res
}

func (self *ApiDescription) verifyPluginCall(
	callsite vfilter.CallSite,
	state *AnalysisState,
	descriptor_lookup map[string]CallDescriptor,
	res []error) []error {
	desc, pres := descriptor_lookup[callsite.Name]
	if !pres {
		return self.verifyLETCall(callsite, state, res)
	}
	// Plugin is found as a regular plugin.

	state.AnalyseCall(callsite, desc)

	for _, arg := range callsite.Args {
		if arg == "**" {
			return self.checkKWArgs(state, callsite, res)
		}

		// If the plugin accepts FreeFormArgs we can call it
		// with any arg but otherwise we can only use a
		// required arg.
		if !desc.FreeFormArgs {
			_, pres := desc.ArgsRequired[arg]
			if !pres {
				res = emitError(INVALID_ARG, state, res,
					INVALID_ARG_FOR_PLUGIN_MSG,
					arg, callsite.Type, callsite.Name)
			}
		}
	}

	// Now check if any of the required args are missing
	for arg, required := range desc.ArgsRequired {
		if bool(required) && !utils.InString(callsite.Args, arg) {
			res = emitError(REQUIRED_ARG_MISSING, state, res,
				REQUIRED_ARG_MISSING_FOR_PLUGIN_MSG,
				arg, callsite.Type, callsite.Name)
		}
	}
	return res
}

func (self *ApiDescription) VerifyCallSite(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository,
	callsite vfilter.CallSite,
	state *AnalysisState) (res []error) {

	err := self.init()
	if err != nil {
		return []error{err}
	}

	if strings.HasPrefix(callsite.Name, "Artifact.") {
		artifact_name := strings.TrimPrefix(callsite.Name, "Artifact.")
		return self.verifyArtifact(ctx, config_obj,
			repository, artifact_name, callsite, state, res)
	}

	// If the callsite contains a . we have no idea what it
	// means. Assume this is not an error. It may be a startlark
	// module for example.
	if strings.Contains(callsite.Name, ".") {
		return nil
	}

	if callsite.Type == "plugin" {
		return self.verifyPluginCall(callsite, state, self.plugins, res)
	}

	if callsite.Type == "function" {
		return self.verifyPluginCall(callsite, state, self.functions, res)
	}

	if callsite.Type == "symbol" {
		return self.verifySymbol(callsite, state, res)
	}

	return res

}

// Run additional validation on the VQL to ensure it is valid.
func VerifyVQL(ctx context.Context, config_obj *config_proto.Config,
	query string, repository services.Repository,
	state *AnalysisState) (res []error) {

	scope := vql_subsystem.MakeScope()

	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return []error{err}
	}

	// In VQL sometimes it is possible to use definition defined after
	// the place of use. We therefore make two passes - one to find
	// the definition the second pass we check for use. We may not
	// find callsite that will fail at runtime but we assume they are
	// available.
	for _, vql := range vqls {
		// Visit the VQL looking for plugin callsites.
		visitor := vfilter.NewVisitor(scope, vfilter.CollectDefinitionSites)
		visitor.Visit(vql)

		for _, def := range visitor.Definitions {
			state.Definitions[def.Name] = def
		}
	}

	for _, vql := range vqls {
		// Visit the VQL looking for plugin callsites.
		visitor := vfilter.NewVisitor(scope, vfilter.CollectCallSites)
		visitor.Visit(vql)

		for _, cs := range visitor.CallSites {
			res = append(res, api_description.VerifyCallSite(
				ctx, config_obj, repository, cs, state)...)
		}
	}

	return res
}

var VQLPaths = []string{
	"sources.[].query",
	"export",
	"precondition",
	"sources.[].precondition",
}

func VerifyArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository,
	artifact *artifacts_proto.Artifact,
	state *AnalysisState) {

	scope := vql_subsystem.MakeScope()

	// First gather all the suppressions from all comments in VQL
	// sections in this artifact.
	gatherSuppressions(scope, state, artifact)

	preamble := ""
	for _, imp := range artifact.Imports {
		dep, pres := repository.Get(ctx, config_obj, imp)
		if !pres {
			state.SetError(
				INVALID_IMPORT, INVALID_IMPORT_MSG,
				imp, artifact.Name)
		} else {
			if dep.Export == "" {
				state.SetError(
					INVALID_IMPORT, INVALID_IMPORT_NO_EXPORT_MSG,
					imp, artifact.Name)

			} else {
				preamble += dep.Export
			}
		}
	}

	if artifact.Precondition != "" {
		for _, err := range VerifyVQL(ctx, config_obj,
			artifact.Precondition, repository, state) {
			state.SetError(
				ARTIFACT_VQL_ERROR, ARTIFACT_VQL_PRECOND_MSG,
				artifact.Name, err)
		}
	}

	if artifact.Export != "" {
		for _, err := range VerifyVQL(ctx, config_obj,
			preamble+artifact.Export, repository, state) {
			state.SetError(
				ARTIFACT_VQL_ERROR, ARTIFACT_VQL_EXPORT_MSG,
				artifact.Name, err)
		}

		preamble += artifact.Export
	}

	for _, s := range artifact.Sources {
		name := artifact.Name
		if s.Name != "" {
			name += "/" + s.Name
		}

		if s.Query != "" {
			dependency := make(map[string]int)

			query := preamble + s.Query

			// The export section if it exists is injection prior to
			// any query.
			err := GetQueryDependencies(ctx, config_obj,
				repository, query, 0, dependency)
			if err != nil {
				state.SetError(
					ARTIFACT_VQL_ERROR, ARTIFACT_VQL_QUERY_MSG,
					name, err)
				continue
			}

			// Now check for broken callsites
			for _, err := range VerifyVQL(ctx, config_obj,
				query, repository, state) {
				state.SetError(
					ARTIFACT_VQL_ERROR, ARTIFACT_VQL_QUERY_MSG, name, err)
			}
		}

		if s.Precondition != "" {
			for _, err := range VerifyVQL(ctx, config_obj,
				s.Precondition, repository, state) {
				state.SetError(ARTIFACT_VQL_ERROR,
					ARTIFACT_VQL_PRECOND_MSG, name, err)
			}
		}
	}

	state.AnalyseArtifactRequiredPermissions(artifact)
}

// Gather the different supporession in different areas of the
// artifact. NOTE: Suppressions declared in any section of the
// artifact apply to the entire artifact.
func gatherSuppressions(
	scope types.Scope, state *AnalysisState, artifact *artifacts_proto.Artifact) {

	gatherSuppressionFromQuery(scope, state, artifact.Precondition)
	gatherSuppressionFromQuery(scope, state, artifact.Export)
	for _, s := range artifact.Sources {
		gatherSuppressionFromQuery(scope, state, s.Precondition)
		gatherSuppressionFromQuery(scope, state, s.Query)
	}
}

func gatherSuppressionFromQuery(
	scope types.Scope, state *AnalysisState, query string) {

	if query == "" {
		return
	}

	vqls, err := vfilter.MultiParseWithComments(query)
	if err != nil {
		return
	}

	for _, vql := range vqls {
		// Visit the VQL looking for plugin callsites.
		visitor := vfilter.NewVisitor(scope, vfilter.CollectComments)
		visitor.Visit(vql)

		state.ParseSuppressions(visitor.Comments)
	}
}

func emitError(name string, state *AnalysisState,
	res []error, message string, args ...interface{}) []error {

	if state.matchSuppression(name, args...) {
		return res
	}

	return append(res, fmt.Errorf(name+":"+message, args...))
}

func emitWarning(name string, state *AnalysisState,
	message string, args ...interface{}) {
	if state.matchSuppression(name, args...) {
		return
	}

	state.Warnings = append(state.Warnings,
		fmt.Sprintf(name+":"+message, args...))
}
