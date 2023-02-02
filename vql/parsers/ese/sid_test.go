package ese

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// https://devblogs.microsoft.com/oldnewthing/20040315-00/?p=40253
func TestSID(t *testing.T) {
	hexsid := "010500000000000515000000A065CF7E784B9B5FE77C8770091C0100"
	sid := formatGUID(hexsid)
	assert.Equal(t, "S-1-5-21-2127521184-1604012920-1887927527-72713", sid)
}
