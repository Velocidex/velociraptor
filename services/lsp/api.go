package lsp

import (
	"context"
	"regexp"
	"strings"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	doc_regex = regexp.MustCompile("doc=(.+)")

	mu                 sync.Mutex
	cachedDescriptions []*api_proto.Completion
	func_lookup        map[string]*api_proto.Completion
)

// Get the top level description line.
func elideDescription(in string) string {
	parts := strings.SplitN(in, ".", 2)
	return utils.Elide(parts[0], 80)
}

func LoadApiDescriptions() []*api_proto.Completion {
	mu.Lock()
	defer mu.Unlock()
	return loadApiDescriptions()
}

func loadApiDescriptions() []*api_proto.Completion {
	if len(cachedDescriptions) > 0 {
		return cachedDescriptions
	}

	descriptions, err := utils.LoadApiDescription()
	if err != nil {
		descriptions = IntrospectDescription()
	}

	for _, d := range descriptions {
		d.Description = elideDescription(d.Description)
	}

	// Cache it for next time.
	cachedDescriptions = descriptions

	return cachedDescriptions
}

func IntrospectDescription() []*api_proto.Completion {
	result := []*api_proto.Completion{}

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := types.NewTypeMap()
	info := scope.Describe(type_map)

	for _, item := range info.Functions {
		var metadata map[string]string
		if item.Metadata != nil {
			metadata = make(map[string]string)
			for _, i := range item.Metadata.Items() {
				metadata[i.Key] = utils.ToString(i.Value)
			}
		}
		result = append(result, &api_proto.Completion{
			Name:        item.Name,
			Description: elideDescription(item.Doc),
			Type:        "Function",
			Args:        getArgDescriptors(item.ArgType, type_map, scope),
			Metadata:    metadata,
		})
	}

	for _, item := range info.Plugins {
		var metadata map[string]string
		if item.Metadata != nil {
			metadata = make(map[string]string)
			for _, i := range item.Metadata.Items() {
				metadata[i.Key] = utils.ToString(i.Value)
			}
		}
		result = append(result, &api_proto.Completion{
			Name:        item.Name,
			Description: elideDescription(item.Doc),
			Type:        "Plugin",
			Args:        getArgDescriptors(item.ArgType, type_map, scope),
			Metadata:    metadata,
		})
	}

	return result
}

func getArgDescriptors(
	arg_type string,
	type_map *vfilter.TypeMap,
	scope vfilter.Scope) []*api_proto.ArgDescriptor {
	args := []*api_proto.ArgDescriptor{}
	arg_desc, pres := type_map.Get(scope, arg_type)
	if pres && arg_desc != nil && arg_desc.Fields != nil {
		for _, i := range arg_desc.Fields.Items() {
			v, ok := i.Value.(*types.TypeReference)
			if !ok {
				continue
			}

			target := v.Target
			if v.Repeated {
				target = " list of " + target
			}

			required := ""
			if strings.Contains(v.Tag, "required") {
				required = "(required)"
			}
			doc := ""
			matches := doc_regex.FindStringSubmatch(v.Tag)
			if matches != nil {
				doc = matches[1]
			}
			args = append(args, &api_proto.ArgDescriptor{
				Name:        i.Key,
				Description: elideDescription(doc) + required,
				Type:        target,
			})
		}
	}
	return args
}

func getArtifactParamDescriptors(
	artifact *artifacts_proto.Artifact) []*api_proto.ArgDescriptor {
	args := []*api_proto.ArgDescriptor{}

	for _, parameter := range artifact.Parameters {
		args = append(args, &api_proto.ArgDescriptor{
			Name:        parameter.Name,
			Description: elideDescription(parameter.Description),
			Type:        "Artifact Parameter",
		})
	}

	return args
}

func LoadArtifactDescriptions(
	ctx context.Context,
	config_obj *config_proto.Config) (
	result []*api_proto.Completion, err error) {

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}
	names, err := repository.List(ctx, config_obj)
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		artifact, pres := repository.Get(ctx, config_obj, name)
		if !pres {
			continue
		}
		result = append(result, &api_proto.Completion{
			Name: "Artifact." + name,
			Type: "Artifact",
			Args: getArtifactParamDescriptors(artifact),
		})
	}
	return result, nil
}
