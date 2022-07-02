package orgs

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"

	"www.velocidex.com/golang/velociraptor/constants"
)

func NewOrgId() string {
	buf := make([]byte, 2)
	_, _ = rand.Read(buf)

	result := base32.HexEncoding.EncodeToString(buf)[:4]
	return constants.ORG_PREFIX + result
}

func NewNonce() string {
	nonce := make([]byte, 8)
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(nonce)
}
