package vtesting

import (
	"testing"

	"github.com/stretchr/testify/require"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetTestConfig(t *testing.T) *config_proto.Config {
	config_obj, err := config.LoadConfig(
		"../http_comms/test_data/server.config.yaml")
	require.NoError(t, err)

	require.NoError(t, config.ValidateFrontendConfig(config_obj))

	config_obj.Datastore.Implementation = "Test"
	config_obj.Frontend.DoNotCompressArtifacts = true

	return config_obj
}
