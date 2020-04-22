package artifacts

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

func GetArtifactSources(
	config_obj *config_proto.Config,
	artifact string) []string {
	result := []string{}
	repository, err := GetGlobalRepository(config_obj)
	if err == nil {
		artifact_obj, pres := repository.Get(artifact)
		if pres {
			for _, source := range artifact_obj.Sources {
				result = append(result, source.Name)
			}
		}
	}
	return result
}
