package lsp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
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
	if err != nil || len(descriptions) == 0 {
		// The embedded reference document is compiled into every
		// build so this should never happen - but if it does we
		// must not fail silently or every IDE feature will appear
		// broken with nothing in the logs to explain it. Note we
		// can not use the regular logger here as it requires a
		// config object we do not carry - stderr is safe because
		// the LSP protocol runs over stdin/stdout.
		fmt.Fprintf(os.Stderr,
			"LSP: unable to load VQL API descriptions: %v\n", err)
	}

	for _, d := range descriptions {
		d.Description = elideDescription(d.Description)
	}

	// Cache it for next time.
	cachedDescriptions = descriptions

	return cachedDescriptions
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
