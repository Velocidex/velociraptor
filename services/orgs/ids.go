package orgs

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"

	"www.velocidex.com/golang/velociraptor/constants"
)

func (self *OrgManager) SetOrgIdForTesting(a string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.NextOrgIdForTesting = &a
}

// Make sure the new ID is unique (There are only 64k possibilities so
// chance of a clash are high)
func (self *OrgManager) NewOrgId() string {
	for {
		buf := make([]byte, 2)
		_, _ = rand.Read(buf)

		org_id := constants.ORG_PREFIX + base32.HexEncoding.EncodeToString(buf)[:4]
		self.mu.Lock()
		if self.NextOrgIdForTesting != nil {
			org_id = *self.NextOrgIdForTesting

			self.mu.Unlock()
			return org_id
		}

		_, pres := self.orgs[org_id]
		self.mu.Unlock()

		if !pres {
			return org_id
		}
	}
}

var (
	NonceForTest = ""
)

func NewNonce() string {
	if NonceForTest != "" {
		return NonceForTest
	}

	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(nonce)
}
