package utils

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"time"

	"www.velocidex.com/golang/velociraptor/constants"
)

var (
	NextFlowIdForTests string
)

func NewFlowId(client_id string) string {
	if NextFlowIdForTests != "" {
		result := NextFlowIdForTests
		NextFlowIdForTests = ""
		return result
	}

	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.FLOW_PREFIX + result
}
