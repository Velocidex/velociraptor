package executor

import (
	"testing"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestMaybeTransformResponseBasicInformation(t *testing.T) {
	identity := &PoolIdentity{
		Hostname: "brave-falcon",
		Fqdn:     "brave-falcon.local",
	}

	response := &actions_proto.VQLResponse{
		Columns:       []string{"Hostname", "Fqdn", "OS"},
		JSONLResponse: "{\"Hostname\":\"real-host\",\"Fqdn\":\"real-host.example.com\",\"OS\":\"linux\"}\n",
	}

	result := maybeTransformResponse(response, identity)
	assert.NotNil(t, result)
	assert.Contains(t, result.JSONLResponse, "brave-falcon")
	assert.Contains(t, result.JSONLResponse, "brave-falcon.local")
	assert.Contains(t, result.JSONLResponse, "linux")
	// The original response must not be mutated.
	assert.Contains(t, response.JSONLResponse, "real-host")
}

func TestMaybeTransformResponseDetailedInfo(t *testing.T) {
	identity := &PoolIdentity{
		Hostname: "cosmic-orbit",
		Fqdn:     "cosmic-orbit.local",
	}

	response := &actions_proto.VQLResponse{
		Columns: []string{"Param", "Value"},
		JSONLResponse: "{\"Param\":\"Hostname\",\"Value\":\"real-host\"}\n" +
			"{\"Param\":\"Fqdn\",\"Value\":\"real-host.example.com\"}\n" +
			"{\"Param\":\"OS\",\"Value\":\"linux\"}\n",
	}

	result := maybeTransformResponse(response, identity)
	assert.NotNil(t, result)
	assert.Contains(t, result.JSONLResponse, "\"Value\":\"cosmic-orbit\"")
	assert.Contains(t, result.JSONLResponse, "\"Value\":\"cosmic-orbit.local\"")
	assert.Contains(t, result.JSONLResponse, "\"Value\":\"linux\"")
}

func TestMaybeTransformResponseNil(t *testing.T) {
	assert.Nil(t, maybeTransformResponse(nil, &PoolIdentity{}))

	// A nil identity means no transformation is applied.
	response := &actions_proto.VQLResponse{
		Columns:       []string{"Hostname"},
		JSONLResponse: "{\"Hostname\":\"real-host\"}\n",
	}
	assert.Equal(t, response, maybeTransformResponse(response, nil))
}
