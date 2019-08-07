/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
package crypto

import (
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TestSuite struct {
	suite.Suite
	config_obj     *api_proto.Config
	server_manager *CryptoManager
	client_manager *CryptoManager
	client_id      string
}

func (self *TestSuite) SetupTest() {
	t := self.T()
	config_obj, err := config.LoadClientConfig(
		"../http_comms/test_data/server.config.yaml")
	require.NoError(t, err)

	utils.Debug(err)

	self.config_obj = config_obj
	self.config_obj.Client.WritebackLinux = ""
	self.config_obj.Client.WritebackWindows = ""
	key, _ := GeneratePrivateKey()
	self.config_obj.Writeback.PrivateKey = string(key)

	// Configure the client manager.
	self.client_manager, err = NewClientCryptoManager(
		self.config_obj, []byte(self.config_obj.Writeback.PrivateKey))
	require.NoError(t, err)

	_, err = self.client_manager.AddCertificate(
		[]byte(self.config_obj.Frontend.Certificate))
	require.NoError(t, err)

	self.client_id = ClientIDFromPublicKey(
		&self.client_manager.private_key.PublicKey)

	// Configure the server manager.
	self.server_manager, err = NewServerCryptoManager(self.config_obj)
	require.NoError(t, err)

	// Install an in memory public key resolver.
	self.server_manager.public_key_resolver = NewInMemoryPublicKeyResolver()
	self.server_manager.public_key_resolver.SetPublicKey(
		self.client_id, &self.client_manager.private_key.PublicKey)
}

func (self *TestSuite) TestEncDecServerToClient() {
	t := self.T()
	message_list := &crypto_proto.MessageList{}
	for i := 0; i < 5; i++ {
		message_list.Job = append(
			message_list.Job, &crypto_proto.GrrMessage{
				Name: "OMG it's a string"})
	}

	serialized, err := proto.Marshal(message_list)
	assert.NoError(t, err)

	cipher_text, err := self.server_manager.Encrypt(
		serialized, self.client_id)
	assert.NoError(t, err)

	initial_c := testutil.ToFloat64(rsaDecryptCounter)

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
	c := testutil.ToFloat64(rsaDecryptCounter)
	assert.Equal(t, c-initial_c, float64(1))
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
		message_list, constants.FRONTEND_NAME)
	assert.NoError(t, err)

	initial_c := testutil.ToFloat64(rsaDecryptCounter)

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
	c := testutil.ToFloat64(rsaDecryptCounter)
	assert.Equal(t, c-initial_c, float64(1))
}

func (self *TestSuite) TestEncryption() {
	t := self.T()
	plain_text := []byte("hello world")

	initial_c := testutil.ToFloat64(rsaDecryptCounter)
	for i := 0; i < 100; i++ {
		cipher_text, err := self.client_manager.Encrypt(
			plain_text, constants.FRONTEND_NAME)
		assert.NoError(t, err)

		result, err := self.server_manager.Decrypt(cipher_text)
		assert.NoError(t, err)

		assert.Equal(t, self.client_id, result.Source)
		assert.Equal(t, result.Authenticated, true)
		assert.Equal(t, result.Raw, plain_text)
	}

	// We should encrypt this only once since we cache the cipher in the output LRU.
	c := testutil.ToFloat64(rsaDecryptCounter)
	assert.Equal(t, c-initial_c, float64(1))
}

func (self *TestSuite) TestClientIDFromPublicKey() {
	t := self.T()

	client_private_key, err := parseRsaPrivateKeyFromPemStr(
		[]byte(self.config_obj.Writeback.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}

	client_id := ClientIDFromPublicKey(&client_private_key.PublicKey)
	assert.True(t, strings.HasPrefix(client_id, "C."))
}

func TestMain(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
