package crypto

import (
	"crypto/rsa"
	"strings"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
)

type publicKeyResolver interface {
	GetPublicKey(subject string) (*rsa.PublicKey, bool)
	SetPublicKey(subject string, key *rsa.PublicKey) error
}

type inMemoryPublicKeyResolver struct {
	public_keys map[string]*rsa.PublicKey
}

func NewInMemoryPublicKeyResolver() publicKeyResolver {
	return &inMemoryPublicKeyResolver{
		public_keys: make(map[string]*rsa.PublicKey),
	}
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
func (self *inMemoryPublicKeyResolver) GetPublicKey(subject string) (*rsa.PublicKey, bool) {
	// GRR sometimes prefixes common names with aff4:/ so strip it first.
	normalized_subject := strings.TrimPrefix(subject, "aff4:/")
	result, pres := self.public_keys[normalized_subject]
	return result, pres
}

func (self *inMemoryPublicKeyResolver) SetPublicKey(
	subject string, key *rsa.PublicKey) error {
	self.public_keys[subject] = key
	return nil
}

type serverPublicKeyResolver struct {
	config_obj *config.Config
	cache      *cache.LRUCache
}

func (self *serverPublicKeyResolver) GetPublicKey(
	client_id string) (*rsa.PublicKey, bool) {
	subject := "aff4:/" + client_id + "/key"
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, false
	}

	data, err := db.GetSubjectAttributes(
		self.config_obj, subject, constants.ATTRS_CLIENT_KEYS)
	if err != nil {
		return nil, false
	}
	pem, pres := data[constants.CLIENT_PUBLIC_KEY]
	if !pres {
		return nil, false
	}

	key, err := PemToPublicKey(pem)
	if err != nil {
		return nil, false
	}

	return key, true
}

func (self *serverPublicKeyResolver) SetPublicKey(
	client_id string, key *rsa.PublicKey) error {
	subject := "aff4:/" + client_id + "/key"

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	data := make(map[string][]byte)
	data[constants.CLIENT_PUBLIC_KEY] = PublicKeyToPem(key)

	return db.SetSubjectData(self.config_obj, subject, 0, data)
}

func NewServerPublicKeyResolver(config_obj *config.Config) publicKeyResolver {
	return &serverPublicKeyResolver{
		config_obj: config_obj,
		cache:      cache.NewLRUCache(1000),
	}
}
