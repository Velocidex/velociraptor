package orgs

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"

	"www.velocidex.com/golang/velociraptor/constants"
)

// Make sure the new ID is unique (There are only 64k possibilities so
// chance of a clash are high)
func (self *OrgManager) NewOrgId() string {
	for {
		buf := make([]byte, 2)
		_, _ = rand.Read(buf)

		org_id := constants.ORG_PREFIX + base32.HexEncoding.EncodeToString(buf)[:4]
		self.mu.Lock()
		_, pres := self.orgs[org_id]
		self.mu.Unlock()

		if !pres {
			return org_id
		}
	}
}

func NewNonce() string {
	nonce := make([]byte, 8)
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(nonce)
}
