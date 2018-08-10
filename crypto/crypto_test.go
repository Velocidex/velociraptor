package crypto

import (
	metrics "github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type TestSuite struct {
	suite.Suite
	config_obj     *config.Config
	server_manager *CryptoManager
	client_manager *CryptoManager
	client_id      string
}

func (self *TestSuite) SetupTest() {
	t := self.T()
	config_obj, err := config.LoadClientConfig("../test_data/server.config.yaml")
	assert.NoError(t, err)

	self.config_obj = config_obj
	self.config_obj.Client.WritebackLinux = ""
	self.config_obj.Client.WritebackWindows = ""

	// Configure the client manager.
	self.client_manager, err = NewClientCryptoManager(
		self.config_obj, []byte(self.config_obj.Writeback.PrivateKey))
	assert.NoError(t, err)
	_, err = self.client_manager.AddCertificate(
		[]byte(self.config_obj.Frontend.Certificate))
	assert.NoError(t, err)

	self.client_id = ClientIDFromPublicKey(
		&self.client_manager.private_key.PublicKey)

	// Configure the server manager.
	self.server_manager, err = NewServerCryptoManager(self.config_obj)
	assert.NoError(t, err)

	// Install an in memory public key resolver.
	self.server_manager.public_key_resolver = NewInMemoryPublicKeyResolver()
	self.server_manager.public_key_resolver.SetPublicKey(
		self.client_id, &self.client_manager.private_key.PublicKey)

	// Clear the metrics for each test case.
	metrics.DefaultRegistry.UnregisterAll()
}

func (self *TestSuite) TestEncDecServerToClient() {
	t := self.T()
	message_list := &crypto_proto.MessageList{}
	for i := 0; i < 5; i++ {
		message_list.Job = append(
			message_list.Job, &crypto_proto.GrrMessage{
				Name: "OMG it's a string"})
	}

	cipher_text, err := self.server_manager.EncryptMessageList(
		message_list, self.client_id)
	assert.NoError(t, err)

	// Decrypt the same message 100 times.
	for i := 0; i < 100; i++ {
		result, err := self.client_manager.DecryptMessageList(cipher_text)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Job {
			assert.Equal(t, item.Name, "OMG it's a string")
			assert.Equal(t, item.AuthState, crypto_proto.GrrMessage_AUTHENTICATED)
		}
	}

	// This should only do the RSA operation once since it should
	// hit the LRU cache each time.
	c := metrics.GetOrRegisterCounter("rsa.decrypt", nil)
	assert.Equal(t, c.Count(), int64(1))
}

func (self *TestSuite) TestEncDecClientToServer() {
	t := self.T()
	message_list := &crypto_proto.MessageList{}
	for i := 0; i < 5; i++ {
		message_list.Job = append(
			message_list.Job, &crypto_proto.GrrMessage{
				Name: "OMG it's a string"})
	}

	cipher_text, err := self.client_manager.EncryptMessageList(
		message_list, "VelociraptorServer")
	assert.NoError(t, err)

	// Decrypt the same message 100 times.
	for i := 0; i < 100; i++ {
		result, err := self.server_manager.DecryptMessageList(cipher_text)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Job {
			assert.Equal(t, item.Name, "OMG it's a string")
			assert.Equal(t, item.AuthState, crypto_proto.GrrMessage_AUTHENTICATED)
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
	destination := "VelociraptorServer"

	for i := 0; i < 100; i++ {
		cipher_text, err := self.client_manager.Encrypt(
			plain_text, destination)
		assert.NoError(t, err)

		result, err := self.server_manager.Decrypt(cipher_text)
		assert.NoError(t, err)

		assert.Equal(t, self.client_id, result.Source)
		assert.Equal(t, result.Authenticated, true)
		assert.Equal(t, result.Raw, plain_text)
	}

	// We should encrypt this only once since we cache the cipher in the output LRU.
	c := metrics.GetOrRegisterCounter("rsa.encrypt", nil)
	assert.Equal(t, c.Count(), int64(1))
}

func (self *TestSuite) TestClientIDFromPublicKey() {
	t := self.T()

	client_private_key, err := parseRsaPrivateKeyFromPemStr(
		[]byte(self.config_obj.Writeback.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, ClientIDFromPublicKey(&client_private_key.PublicKey),
		"C.5416094c54e066be")
}

func TestMain(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
