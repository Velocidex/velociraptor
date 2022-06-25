package orgs

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"time"

	"www.velocidex.com/golang/velociraptor/constants"
)

func NewOrgId() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.ORG_PREFIX + result
}

func NewNonce() string {
	nonce := make([]byte, 8)
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(nonce)
}
