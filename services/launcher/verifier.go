package launcher

import (
	"context"
	"fmt"
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	api_description = &ApiDescription{}
)

// Contains the result of the static analysis.
type AnalysisState struct {
	Artifact    string
	Permissions []string
	Errors      []error
	Warnings    []string
}

func (self *AnalysisState) SetError(err error) {
	self.Errors = append(self.Errors, err)
}

func (self *AnalysisState) AnalyseCall(
	callsite vfilter.CallSite, desc CallDesciptor) {
	self.Permissions = utils.Sort(utils.DeduplicateStringSlice(
		append(self.Permissions, desc.Permissions...)))
}

func (self *AnalysisState) AnalyseArtifactRequiredPermissions(
	artifact *artifacts_proto.Artifact) {
	switch strings.ToUpper(artifact.Type) {
	case "", "CLIENT":
	default:
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
			self.Warnings = append(self.Warnings,
				fmt.Sprintf("<yellow>Suggestion</>: Add %v to artifact's required_permissions or implied_permissions fields", perm))
		}
	}
}

func NewAnalysisState(artifact string) *AnalysisState {
	return &AnalysisState{
		Artifact: artifact,
	}
}

type Required bool

type CallDesciptor struct {
	ArgsRequired map[string]Required
	Permissions  []string
}

func (self *CallDesciptor) SetPermissions(api *api_proto.Completion) {
	if api.Metadata != nil {
		perms, pres := api.Metadata["permissions"]
		if pres {
			self.Permissions = strings.Split(perms, ",")
		}
	}
}

func NewCallDesciptor(api *api_proto.Completion) CallDesciptor {
	res := &CallDesciptor{
		ArgsRequired: make(map[string]Required),
	}

	res.SetPermissions(api)
	return *res
}

type ApiDescription struct {
	functions map[string]CallDesciptor
	plugins   map[string]CallDesciptor
}

func (self *ApiDescription) init() error {
	// Initialize if needed
	if self.functions == nil || self.plugins == nil {
		self.functions = make(map[string]CallDesciptor)
		self.plugins = make(map[string]CallDesciptor)

		apis, err := utils.LoadApiDescription()
		if err != nil {
			return err
		}

		for _, api := range apis {
			if api.Type == "Function" {
				desc := NewCallDesciptor(api)

				// Ignore ** kwargs type of call.
				desc.ArgsRequired["**"] = Required(false)
				for _, arg := range api.Args {
					desc.ArgsRequired[arg.Name] = Required(arg.Required)
				}
				if !api.FreeFormArgs {
					self.functions[api.Name] = desc
				}
			}
			if api.Type == "Plugin" {
				desc := NewCallDesciptor(api)

				// Ignore ** kwargs type of call.
				desc.ArgsRequired["**"] = Required(false)
				for _, arg := range api.Args {
					desc.ArgsRequired[arg.Name] = Required(arg.Required)
				}
				if !api.FreeFormArgs {
					self.plugins[api.Name] = desc
				}
			}
		}
	}
	return nil
}

func (self *ApiDescription) verifyArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository, artifact_name string,
	callsite vfilter.CallSite) (res []error) {

	artifact, pres := repository.Get(ctx, config_obj, artifact_name)
	if !pres {
		return []error{fmt.Errorf("Query calls Unknown artifact %v", artifact_name)}
	}

	parameters := make(map[string]bool)

	// Some implicit parameters that are always allowed
	parameters["source"] = true
	parameters["preconditions"] = true

	for _, p := range artifact.Parameters {
		parameters[p.Name] = true
	}

	for _, arg := range callsite.Args {
		_, pres := parameters[arg]
		if !pres {
			res = append(res, fmt.Errorf("Call to %v contains unknown parameter %v",
				callsite.Name, arg))
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
			repository, artifact_name, callsite)
	}

	if callsite.Type == "plugin" {
		desc, pres := self.plugins[callsite.Name]
		if pres {
			state.AnalyseCall(callsite, desc)

			for _, arg := range callsite.Args {
				_, pres := desc.ArgsRequired[arg]
				if !pres {
					res = append(res, fmt.Errorf(
						"Invalid arg %v for plugin %v()",
						arg, callsite.Name))
				}
			}

			// Now check if any of the required args are missing
			for arg, required := range desc.ArgsRequired {
				if bool(required) && !utils.InString(callsite.Args, arg) {
					res = append(res, fmt.Errorf(
						"While calling plugin %v(), required arg %v is not provided",
						callsite.Name, arg))
				}
			}
		}
	}

	if callsite.Type == "function" {
		desc, pres := self.functions[callsite.Name]
		if pres {
			state.AnalyseCall(callsite, desc)

			for _, arg := range callsite.Args {
				_, pres := desc.ArgsRequired[arg]
				if !pres {
					res = append(res, fmt.Errorf(
						"Invalid arg %v for function %v()",
						arg, callsite.Name))
				}
			}

			// Now check if any of the required args are missing
			for arg, required := range desc.ArgsRequired {
				if bool(required) && !utils.InString(callsite.Args, arg) {
					res = append(res, fmt.Errorf(
						"While calling vql function %v(), required arg %v is not called",
						callsite.Name, arg))
				}
			}
		}
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

func VerifyArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository,
	artifact *artifacts_proto.Artifact,
	state *AnalysisState) {

	if artifact.Precondition != "" {
		for _, err := range VerifyVQL(ctx, config_obj,
			artifact.Precondition, repository, state) {
			state.SetError(err)
		}
	}

	if artifact.Export != "" {
		for _, err := range VerifyVQL(ctx, config_obj,
			artifact.Export, repository, state) {
			state.SetError(err)
		}
	}

	for _, s := range artifact.Sources {
		if s.Query != "" {
			dependency := make(map[string]int)

			err := GetQueryDependencies(ctx, config_obj,
				repository, s.Query, 0, dependency)
			if err != nil {
				state.SetError(err)
				continue
			}

			// Now check for broken callsites
			for _, err := range VerifyVQL(ctx, config_obj,
				s.Query, repository, state) {
				state.SetError(err)
			}
		}
		if s.Precondition != "" {
			for _, err := range VerifyVQL(ctx, config_obj,
				s.Precondition, repository, state) {
				state.SetError(err)
			}
		}
	}

	state.AnalyseArtifactRequiredPermissions(artifact)
}
