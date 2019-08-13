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
			PrivateKey: "hello",
		},
	}

	obfuscator := &Obfuscator{}
	obf, err := obfuscator.Encrypt(config_obj, name)
	assert.NoError(t, err, "Cant encrypt")
	plain, err := obfuscator.Decrypt(config_obj, obf)
	assert.NoError(t, err, "Cant decrypt")
	assert.Equal(t, plain, name)
}
