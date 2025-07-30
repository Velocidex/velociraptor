package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// A global DEK
	mu  sync.Mutex
	dek []byte
)

// Gets the Data Encryption Key. This is derived from the config file
// or obtained from the environment.
func GetDek(ctx context.Context, config_obj *config_proto.Config) ([]byte, error) {
	mu.Lock()
	defer mu.Unlock()

	if len(dek) != 0 {
		return dek, nil
	}

	if config_obj.Frontend == nil ||
		config_obj.Security == nil ||
		len(config_obj.Frontend.PrivateKey) == 0 {
		return nil, utils.InvalidConfigError
	}

	// Set the global DEK from the config file.
	dek_str := config_obj.Security.SecretsDek
	if dek_str == "" {
		dek_str = config_obj.ObfuscationNonce
	}

	if dek_str == "" {
		dek_str = config_obj.Frontend.PrivateKey
	}

	// Should we get it from the environment?
	env_var := strings.TrimPrefix(dek_str, "env://")
	if env_var != dek_str {
		dek_str = os.Getenv(env_var)
	}

	// The key is the hash of the dek - this is needed to ensure the
	// key is long enough
	sha_sum := sha256.New()
	_, _ = sha_sum.Write([]byte(dek_str))
	dek = sha_sum.Sum(nil)

	return dek, nil
}

func encrypt(dek, plaintext []byte) ([]byte, error) {
	aes, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(aes)
	if err != nil {
		return nil, err
	}

	// We need a 12-byte nonce for GCM (modifiable if you use
	// cipher.NewGCMWithNonceSize()) A nonce should always be randomly
	// generated for every encryption.
	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		return nil, err
	}

	// ciphertext here is actually nonce+ciphertext So that when we
	// decrypt, just knowing the nonce size is enough to separate it
	// from the ciphertext.
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return ciphertext, nil
}

func decrypt(dek, ciphertext []byte) ([]byte, error) {
	aes, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(aes)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) <= nonceSize {
		return nil, utils.InvalidArgError
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, []byte(nonce), []byte(ciphertext), nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func PrepareForStorage(
	ctx context.Context,
	config_obj *config_proto.Config,
	secret *services.Secret) (*api_proto.Secret, error) {
	res := proto.Clone(secret.Secret).(*api_proto.Secret)
	serialized, err := json.Marshal(secret.Data)
	if err != nil {
		return nil, err
	}

	dek, err := GetDek(ctx, config_obj)
	if err != nil {
		return nil, err
	}

	res.EncryptedSecret, err = encrypt(dek, serialized)

	// Remove the actual clear text data.
	res.Secret = nil

	return res, err
}

// Decode a secret from the protobuf that carries it. The secret is
// encrypted in the protobuf so we need to unlock it first.
func NewSecretFromProto(
	ctx context.Context,
	config_obj *config_proto.Config,
	secret *api_proto.Secret) (*services.Secret, error) {

	result := &services.Secret{
		Secret: proto.Clone(secret).(*api_proto.Secret),
		Data:   ordereddict.NewDict(),
	}

	// The secret is encrypted, we need to decrypt it.
	if len(result.Secret.EncryptedSecret) > 0 {
		dek, err := GetDek(ctx, config_obj)
		if err != nil {
			return nil, err
		}

		decrypted, err := decrypt(dek, result.Secret.EncryptedSecret)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(decrypted, result.Data)
		if err != nil {
			return nil, err
		}

		result.Secret.EncryptedSecret = nil

		return result, nil
	}

	for k, v := range result.Secret.Secret {
		result.Data.Set(k, v)
	}
	return result, nil
}
