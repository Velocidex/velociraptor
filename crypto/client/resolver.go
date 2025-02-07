/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package client

import (
	"crypto/rsa"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type PublicKeyResolver interface {
	GetPublicKey(
		config_obj *config_proto.Config, subject string) (*rsa.PublicKey, bool)
	SetPublicKey(
		config_obj *config_proto.Config, subject string, key *rsa.PublicKey) error

	// Clear from cache.
	DeleteSubject(subject string)

	Clear() // Flush all internal caches.
}

type inMemoryPublicKeyResolver struct {
	mu          sync.Mutex
	public_keys map[string]*rsa.PublicKey
}

func NewInMemoryPublicKeyResolver() PublicKeyResolver {
	return &inMemoryPublicKeyResolver{
		public_keys: make(map[string]*rsa.PublicKey),
	}
}

func (self *inMemoryPublicKeyResolver) DeleteSubject(subject string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.public_keys, subject)
}

/*
This method can be overridden by derived classes to provide a way of
recovering the public key of each source. We use this public key to:

 1. Verify the message signature when receiving a message from a
    particular source.

 2. Encrypt the message using the public key when encrypting a
    message destined to a particular entity.

Implementations are expected to provide a mapping between known
sources and their public keys.
*/
func (self *inMemoryPublicKeyResolver) GetPublicKey(
	config_obj *config_proto.Config, subject string) (*rsa.PublicKey, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result, pres := self.public_keys[subject]
	return result, pres
}

func (self *inMemoryPublicKeyResolver) SetPublicKey(
	config_obj *config_proto.Config, subject string, key *rsa.PublicKey) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.public_keys[subject] = key
	return nil
}

func (self *inMemoryPublicKeyResolver) Clear() {
	self.public_keys = make(map[string]*rsa.PublicKey)
}
