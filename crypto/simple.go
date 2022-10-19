package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	errors "github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Simple aes obfuscation. This is used in VQL obfuscation and so must
// only contain identifier safe characters. We therefore hex encode
// the encrypted names.
type Obfuscator struct {
	mu      sync.Mutex
	key     []byte
	crypter cipher.Block
}

func (self *Obfuscator) Encrypt(config_obj *config_proto.Config, name string) (
	string, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.key == nil {
		err := self.generateCrypter(config_obj)
		if err != nil {
			return "", err
		}
	}

	plain_text := []byte(name)
	padding := aes.BlockSize - (len(plain_text) % aes.BlockSize)
	for i := 0; i < padding; i++ {
		plain_text = append(plain_text, byte(padding))
	}

	mode := cipher.NewCBCEncrypter(self.crypter, self.key[:aes.BlockSize])
	cipher_text := make([]byte, len(plain_text))
	mode.CryptBlocks(cipher_text, plain_text)

	return "$" + hex.EncodeToString(cipher_text), nil
}

func (self *Obfuscator) generateCrypter(config_obj *config_proto.Config) error {
	hash := sha256.Sum256([]byte(config_obj.ObfuscationNonce))
	self.key = hash[:]
	crypter, err := aes.NewCipher(self.key)
	if err != nil {
		return errors.Wrap(err, 0)
	}
	self.crypter = crypter
	return nil
}

func (self *Obfuscator) Decrypt(config_obj *config_proto.Config, name string) (
	string, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if name == "" {
		return "", nil
	}

	// Not obfuscated
	if name[0] != '$' {
		return name, nil
	}

	cipher_text, err := hex.DecodeString(name[1:])
	if err != nil {
		return "", err
	}

	if len(cipher_text) < 16 || len(cipher_text)%16 != 0 {
		return "", errors.New("Cipher error")
	}

	if self.key == nil {
		err := self.generateCrypter(config_obj)
		if err != nil {
			return "", err
		}
	}

	mode := cipher.NewCBCDecrypter(self.crypter, self.key[:aes.BlockSize])
	plain_text := make([]byte, len(cipher_text))
	mode.CryptBlocks(plain_text, cipher_text)

	padding := int(plain_text[len(plain_text)-1])

	// Invalid padding will occur when the crypto key has changed
	// between the obfuscation and deobfuscation.
	if padding < 0 || padding > 16 {
		return "", errors.New("Padding error")
	}

	for i := len(plain_text) - padding; i < len(plain_text); i++ {
		if int(plain_text[i]) != padding {
			return "", errors.New("Padding error")
		}
	}

	return string(plain_text[:len(plain_text)-padding]), nil
}
