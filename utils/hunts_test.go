package utils

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestExtractHuntId(t *testing.T) {
	hunt_id := "H.123"
	flow_id := CreateFlowIdFromHuntId(hunt_id)
	assert.Equal(t, "F.123.H", flow_id)

	extracted_hunt_id, ok := ExtractHuntId(flow_id)
	assert.True(t, ok)
	assert.Equal(t, extracted_hunt_id, hunt_id)

	// Regular flow
	extracted_hunt_id, ok = ExtractHuntId("F.1234")
	assert.True(t, !ok)
}
