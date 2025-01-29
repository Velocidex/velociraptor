package launcher

import (
	"context"
	"fmt"
	"strings"

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

type Required bool

type ApiDescription struct {
	functions map[string]map[string]Required
	plugins   map[string]map[string]Required
}

func (self *ApiDescription) init() error {
	// Initialize if needed
	if self.functions == nil || self.plugins == nil {
		self.functions = make(map[string]map[string]Required)
		self.plugins = make(map[string]map[string]Required)

		apis, err := utils.LoadApiDescription()
		if err != nil {
			return err
		}

		for _, api := range apis {
			if api.Type == "Function" {
				desc := make(map[string]Required)
				// Ignore ** kwargs type of call.
				desc["**"] = Required(false)
				for _, arg := range api.Args {
					desc[arg.Name] = Required(arg.Required)
				}
				if !api.FreeFormArgs {
					self.functions[api.Name] = desc
				}
			}
			if api.Type == "Plugin" {
				desc := make(map[string]Required)
				desc["**"] = Required(false)
				for _, arg := range api.Args {
					desc[arg.Name] = Required(arg.Required)
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
			res = append(res, fmt.Errorf("Call to %v contain unknown parameter %v",
				callsite.Name, arg))
		}
	}

	return res
}

func (self *ApiDescription) VerifyCallSite(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository,
	callsite vfilter.CallSite) (res []error) {

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
			for _, arg := range callsite.Args {
				_, pres := desc[arg]
				if !pres {
					res = append(res, fmt.Errorf(
						"Invalid arg %v for plugin %v()",
						arg, callsite.Name))
				}
			}

			// Now check if any of the required args are missing
			for arg, required := range desc {
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
			for _, arg := range callsite.Args {
				_, pres := desc[arg]
				if !pres {
					res = append(res, fmt.Errorf(
						"Invalid arg %v for function %v()",
						arg, callsite.Name))
				}
			}

			// Now check if any of the required args are missing
			for arg, required := range desc {
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
	query string, repository services.Repository) (res []error) {

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
				ctx, config_obj, repository, cs)...)
		}
	}

	return res
}

func VerifyArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	artifact_path string,
	artifact *artifacts_proto.Artifact,
	returned_errs map[string]error) {

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return
	}

	if artifact.Precondition != "" {
		for _, err := range VerifyVQL(ctx, config_obj,
			artifact.Precondition, repository) {
			returned_errs[artifact_path] = err
		}
	}

	for _, s := range artifact.Sources {
		if s.Query != "" {
			dependency := make(map[string]int)

			err := GetQueryDependencies(ctx, config_obj,
				repository, s.Query, 0, dependency)
			if err != nil {
				returned_errs[artifact_path] = err
				continue
			}

			// Now check for broken callsites
			for _, err := range VerifyVQL(ctx, config_obj,
				s.Query, repository) {
				returned_errs[artifact_path] = err
			}
		}
		if s.Precondition != "" {
			for _, err := range VerifyVQL(ctx, config_obj,
				s.Precondition, repository) {
				returned_errs[artifact_path] = err
			}
		}
	}
}
