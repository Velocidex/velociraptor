package client

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

var (
	RsaSignCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsa_sign_op",
		Help: "Total number of rsa signatures.",
	})

	RsaEncryptCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsa_encrypt_op",
		Help: "Total number of rsa encryption ops.",
	})

	RsaDecryptCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsa_decrypt_op",
		Help: "Total number of rsa decryption ops.",
	})

	RsaVerifyCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsa_verify_op",
		Help: "Total number of rsa verify ops.",
	})
)

type _Cipher struct {
	key_size                  int
	source                    string
	cipher_properties         *crypto_proto.CipherProperties
	cipher_metadata           *crypto_proto.CipherMetadata
	encrypted_cipher          []byte
	encrypted_cipher_metadata []byte
	authenticated             bool
}

func (self *_Cipher) Size() int {
	return 1
}

func (self *_Cipher) CipherProperties() *crypto_proto.CipherProperties {
	return self.cipher_properties
}

func (self *_Cipher) ClientCommunication() *crypto_proto.ClientCommunication {
	return &crypto_proto.ClientCommunication{
		EncryptedCipher:         self.encrypted_cipher,
		EncryptedCipherMetadata: self.encrypted_cipher_metadata,
		PacketIv:                make([]byte, self.key_size/8),
		ApiVersion:              constants.CLIENT_API_VERSION,
	}
}

func NewCipher(
	source string,
	private_key *rsa.PrivateKey,
	public_key *rsa.PublicKey) (*_Cipher, error) {

	result := &_Cipher{
		source:   source,
		key_size: 128,
	}
	result.cipher_properties = &crypto_proto.CipherProperties{
		Name:       "aes_128_cbc",
		Key:        make([]byte, result.key_size/8),
		MetadataIv: make([]byte, result.key_size/8),
		HmacKey:    make([]byte, result.key_size/8),
		HmacType:   crypto_proto.CipherProperties_FULL_HMAC,
	}

	_, err := rand.Read(result.cipher_properties.Key)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	_, err = rand.Read(result.cipher_properties.MetadataIv)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	_, err = rand.Read(result.cipher_properties.HmacKey)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	result.cipher_metadata = &crypto_proto.CipherMetadata{
		Source: source,
	}

	serialized_cipher, err := proto.Marshal(result.cipher_properties)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	hashed := sha256.Sum256(serialized_cipher)
	RsaSignCounter.Inc()
	signature, err := rsa.SignPKCS1v15(
		rand.Reader, private_key, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}
	result.cipher_metadata.Signature = signature

	RsaEncryptCounter.Inc()
	encrypted_cipher, err := rsa.EncryptOAEP(
		sha1.New(), rand.Reader,
		public_key,
		serialized_cipher, []byte(""))
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	result.encrypted_cipher = encrypted_cipher

	serialized_cipher_metadata, err := proto.Marshal(result.cipher_metadata)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	encrypted_cipher_metadata, err := EncryptSymmetric(
		result.cipher_properties,
		serialized_cipher_metadata,
		result.cipher_properties.MetadataIv)
	if err != nil {
		return nil, err
	}
	result.encrypted_cipher_metadata = encrypted_cipher_metadata

	return result, nil
}
