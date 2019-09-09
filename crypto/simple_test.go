package crypto

import (
	"testing"

	assert "github.com/stretchr/testify/assert"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func TestObfuscation(t *testing.T) {
	name := "Foobar"
	config_obj := &config_proto.Config{
		Frontend: &config_proto.FrontendConfig{
			// The obfuscation key is simply the hash of
			// the PEM private key.
			PrivateKey: "hello",
		},
	}

	obfuscator := &Obfuscator{}

	// Decrypting a string encrypted with another key should
	// error.
	test_str := "$3f6d20f47d3a66dc1e74378916882a74899f0503ec650795d284403286b9fd79"
	_, err := obfuscator.Decrypt(config_obj, test_str)
	assert.Error(t, err)

	// Not hex encoded.
	_, err = obfuscator.Decrypt(config_obj,
		"$3")
	assert.Error(t, err)

	// Block cipher too short
	_, err = obfuscator.Decrypt(config_obj,
		"$3f")
	assert.Error(t, err)

	// Make sure we can obfuscate a string of any length.
	for i := 0; i < 16; i++ {
		obf, err := obfuscator.Encrypt(config_obj, name)
		assert.NoError(t, err, "Cant encrypt")
		plain, err := obfuscator.Decrypt(config_obj, obf)
		assert.NoError(t, err, "Cant decrypt")
		assert.Equal(t, plain, name)

		name = name + "X"
	}
}
