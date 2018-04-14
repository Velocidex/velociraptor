package crypto

import (
	metrics "github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/context"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

type TestSuite struct {
	suite.Suite
	manager *CryptoManager
}

func (self *TestSuite) SetupTest() {
	t := self.T()
	var err error
	ctx := context.Background()
	self.manager, err = NewCryptoManager(
		&ctx,
		"GRR Test Server",
		utils.ReadFile(t, "test_data/server-priv.pem"))
	if err != nil {
		t.Fatal(err)
	}

	err = self.manager.AddCertificate(
		utils.ReadFile(t, "test_data/cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	err = self.manager.AddCertificate(
		utils.ReadFile(t, "test_data/server-priv.pem"))
	if err != nil {
		t.Fatal(err)
	}

	// Clear the metrics for each test case.
	metrics.DefaultRegistry.UnregisterAll()
}

func (self *TestSuite) TestDecryption() {
	t := self.T()
	cipher_text := utils.ReadFile(t, "test_data/enc_message.bin")

	// Decrypt the same message 100 times.
	for i := 0; i < 100; i++ {
		result, err := self.manager.DecryptMessageList(cipher_text)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Job {
			assert.Equal(t, *item.Name, "OMG it's a string")
			assert.Equal(t, *item.AuthState, crypto_proto.GrrMessage_AUTHENTICATED)
		}
	}

	// This should only do the RSA operation once since it should
	// hit the LRU cache each time.
	c := metrics.GetOrRegisterCounter("rsa.decrypt", nil)
	assert.Equal(t, c.Count(), int64(1))
}

func (self *TestSuite) TestEncryption() {
	t := self.T()
	plain_text := []byte("hello world")
	destination := "GRR Test Server"

	for i := 0; i < 100; i++ {
		cipher_text, err := self.manager.Encrypt(
			plain_text,
			destination,
		)
		if err != nil {
			t.Fatal(err)
		}

		result, err := self.manager.Decrypt(cipher_text)
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, destination, *result.Source)
		assert.Equal(t, result.Authenticated, true)
		assert.Equal(t, result.Raw, plain_text)
	}

	// We should encrypt this only once since we cache the cipher in the output LRU.
	c := metrics.GetOrRegisterCounter("rsa.encrypt", nil)
	assert.Equal(t, c.Count(), int64(1))
}

func TestMain(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
