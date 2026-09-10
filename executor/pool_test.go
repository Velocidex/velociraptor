package executor

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Test that the hostname transform works when the response is
// compressed by the launcher (the default for newer clients). The
// transform must decompress the payload, apply the change and return
// the result uncompressed - the server falls back to handling
// uncompressed data.
func TestMaybeTransformResponseCompressed(t *testing.T) {
	identity := &PoolIdentity{
		Hostname: "brave-falcon",
		Fqdn:     "brave-falcon.local",
	}

	rows := []*ordereddict.Dict{
		ordereddict.NewDict().
			Set("Hostname", "testhost").
			Set("Fqdn", "testhost.local"),
	}
	jsonl, err := json.MarshalJsonl(rows)
	require.NoError(t, err)

	compressed, err := utils.Compress(jsonl)
	require.NoError(t, err)

	response := &actions_proto.VQLResponse{
		Columns:                []string{"Hostname", "Fqdn"},
		CompressedJsonResponse: compressed,
		UncompressedSize:       uint64(len(jsonl)),
	}

	result := maybeTransformResponse(response, identity)
	require.NotNil(t, result)

	// The result must be uncompressed.
	assert.NotEmpty(t, result.JSONLResponse)
	assert.Empty(t, result.CompressedJsonResponse)
	assert.Equal(t, uint64(0), result.UncompressedSize)

	transformed_rows, err := utils.ParseJsonToDicts(
		[]byte(result.JSONLResponse))
	require.NoError(t, err)
	require.Len(t, transformed_rows, 1)

	hostname, pres := transformed_rows[0].GetString("Hostname")
	require.True(t, pres)
	assert.Equal(t, "brave-falcon", hostname)

	fqdn, pres := transformed_rows[0].GetString("Fqdn")
	require.True(t, pres)
	assert.Equal(t, "brave-falcon.local", fqdn)
}

// Test that the hostname transform still works on uncompressed
// responses (older clients).
func TestMaybeTransformResponseUncompressed(t *testing.T) {
	identity := &PoolIdentity{
		Hostname: "brave-falcon",
		Fqdn:     "brave-falcon.local",
	}

	rows := []*ordereddict.Dict{
		ordereddict.NewDict().
			Set("Hostname", "testhost").
			Set("Fqdn", "testhost.local"),
	}
	jsonl, err := json.MarshalJsonl(rows)
	require.NoError(t, err)

	response := &actions_proto.VQLResponse{
		Columns:       []string{"Hostname", "Fqdn"},
		JSONLResponse: string(jsonl),
	}

	result := maybeTransformResponse(response, identity)
	require.NotNil(t, result)

	// The result must remain uncompressed.
	assert.Empty(t, result.CompressedJsonResponse)
	assert.NotEmpty(t, result.JSONLResponse)

	transformed_rows, err := utils.ParseJsonToDicts(
		[]byte(result.JSONLResponse))
	require.NoError(t, err)
	require.Len(t, transformed_rows, 1)

	hostname, pres := transformed_rows[0].GetString("Hostname")
	require.True(t, pres)
	assert.Equal(t, "brave-falcon", hostname)
}

// Test that responses without a Hostname column are returned
// unchanged.
func TestMaybeTransformResponseNoHostname(t *testing.T) {
	identity := &PoolIdentity{
		Hostname: "brave-falcon",
		Fqdn:     "brave-falcon.local",
	}

	rows := []*ordereddict.Dict{
		ordereddict.NewDict().Set("Foo", "bar"),
	}
	jsonl, err := json.MarshalJsonl(rows)
	require.NoError(t, err)

	compressed, err := utils.Compress(jsonl)
	require.NoError(t, err)

	response := &actions_proto.VQLResponse{
		Columns:                []string{"Foo"},
		CompressedJsonResponse: compressed,
		UncompressedSize:       uint64(len(jsonl)),
	}

	result := maybeTransformResponse(response, identity)
	require.NotNil(t, result)
	assert.Equal(t, response, result)
}
