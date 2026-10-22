package paths

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func extractClientIdFlowId(in api.FSPathSpec) *ordereddict.Dict {
	components := in.Components()
	return ordereddict.NewDict().
		Set("ClientId", components[1]).
		Set("FlowId", components[3])
}

func extractClientId(in api.FSPathSpec) *ordereddict.Dict {
	components := in.Components()
	return ordereddict.NewDict().
		Set("ClientId", components[1])
}

func extractClientIdFlowIdArtifactName(in api.FSPathSpec) *ordereddict.Dict {
	components := in.Components()
	res := ordereddict.NewDict().
		Set("ClientId", components[1]).
		Set("FlowId", components[4])

	if len(components) == 5 {
		res.Set("Artifact", components[3])
	} else if len(components) == 6 {
		res.Set("Artifact", components[3]+"/"+components[5])
	}
	return res
}

func extractMonitoringClientIdArtifactName(in api.FSPathSpec) *ordereddict.Dict {
	components := in.Components()
	res := ordereddict.NewDict().
		Set("ClientId", components[1])

	if len(components) == 5 {
		res.Set("Artifact", components[3])
	} else if len(components) == 6 {
		res.Set("Artifact", components[3]+"/"+components[4])
	}
	return res
}

func extractMonitoringServerArtifactName(in api.FSPathSpec) *ordereddict.Dict {
	components := in.Components()
	res := ordereddict.NewDict().
		Set("ClientId", "server")

	if len(components) == 3 {
		res.Set("Artifact", components[1])
	} else if len(components) == 4 {
		res.Set("Artifact", components[1]+"/"+components[2])
	}
	return res
}
