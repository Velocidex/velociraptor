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
package crypto_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"sync"
	"testing"
	"time"

	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_server "www.velocidex.com/golang/velociraptor/crypto/server"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TestSuite struct {
	suite.Suite
	config_obj     *config_proto.Config
	server_manager *crypto_server.ServerCryptoManager
	client_manager *crypto_client.ClientCryptoManager

	client_private_key *rsa.PrivateKey
	server_private_key *rsa.PrivateKey
	client_id          string
}

func (self *TestSuite) SetupTest() {
	t := self.T()
	config_obj, err := new(config.Loader).WithFileLoader(
		"../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(t, err)

	self.config_obj = config_obj
	self.config_obj.Client.WritebackLinux = ""
	self.config_obj.Client.WritebackWindows = ""
	key, _ := crypto_utils.GeneratePrivateKey()
	self.config_obj.Writeback.PrivateKey = string(key)

	// Configure the client manager.
	self.client_manager, err = crypto_client.NewClientCryptoManager(
		self.config_obj, []byte(self.config_obj.Writeback.PrivateKey))
	require.NoError(t, err)

	self.client_private_key, err = crypto_utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(self.config_obj.Writeback.PrivateKey))
	require.NoError(self.T(), err)

	self.server_private_key, err = crypto_utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(self.config_obj.Frontend.PrivateKey))
	require.NoError(self.T(), err)

	_, err = self.client_manager.AddCertificate(
		[]byte(self.config_obj.Frontend.Certificate))
	require.NoError(t, err)

	private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(key)
	assert.NoError(t, err)

	self.client_id = crypto_utils.ClientIDFromPublicKey(&private_key.PublicKey)

	// Configure the server manager.
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	self.server_manager, err = crypto_server.NewServerCryptoManager(
		ctx, self.config_obj, wg)
	require.NoError(t, err)

	// Install an in memory public key resolver.
	self.server_manager.Resolver = crypto_client.NewInMemoryPublicKeyResolver()
	self.server_manager.Resolver.SetPublicKey(
		self.client_id, &private_key.PublicKey)
}

func (self *TestSuite) TestEncDecServerToClient() {
	t := self.T()
	message_list := &crypto_proto.MessageList{}
	for i := 0; i < 5; i++ {
		message_list.Job = append(
			message_list.Job, &crypto_proto.VeloMessage{
				Name: "OMG it's a string"})
	}

	serialized, err := proto.Marshal(message_list)
	assert.NoError(t, err)

	cipher_text, err := self.server_manager.Encrypt(
		[][]byte{serialized},
		crypto_proto.PackedMessageList_ZCOMPRESSION,
		self.client_id)
	assert.NoError(t, err)

	initial_c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)

	// Decrypt the same message 100 times.
	for i := 0; i < 100; i++ {
		message_info, err := self.client_manager.Decrypt(cipher_text)
		if err != nil {
			t.Fatal(err)
		}
		message_info.IterateJobs(context.Background(),
			func(ctx context.Context, item *crypto_proto.VeloMessage) {
				assert.Equal(t, item.Name, "OMG it's a string")
				assert.Equal(t, item.AuthState, crypto_proto.VeloMessage_AUTHENTICATED)
			})
	}

	// This should only do the RSA operation once since it should
	// hit the LRU cache each time.
	c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)
	assert.Equal(t, c-initial_c, float64(1))
}

func (self *TestSuite) TestEncDecClientToServerWithSpoof() {
	// The source may be present in two places:

	// 1. The cipher_metadata.Source is verified by the encryption
	//    itself (client id is derived from hash of public key).
	// 2. The entire message is packed into a MessageInfo
	t := self.T()
	message_list := &crypto_proto.MessageList{}
	message_list.Job = append(
		message_list.Job, &crypto_proto.VeloMessage{
			// Spoof the source in the actual message - this will be
			// corrected to the real client id.
			Source: "C.1234Spoof",
			Name:   "OMG it's a string"})

	ConfigObj := config.GetDefaultConfig()
	cipher_text, err := self._EncryptMessageListWithSpoofedPackedMessage(
		message_list, ConfigObj.Client.PinnedServerName)
	assert.NoError(t, err)

	message_info, err := self.server_manager.Decrypt(cipher_text)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, message_info.Source, self.client_id)
	err = message_info.IterateJobs(context.Background(),
		func(ctx context.Context, msg *crypto_proto.VeloMessage) {
			// Make sure the spoofed source is ignored, and the
			// correct source is relayed in the VeloMessage.
			assert.Equal(t, msg.Source, self.client_id)
		})
	assert.NoError(t, err)
}

// Encrypt the messages but deliberately spoofed the client id in the
// PackedMessageList inner message. This should be caught by the
// decryption and not allowed.
func (self *TestSuite) _EncryptMessageListWithSpoofedPackedMessage(
	message_list *crypto_proto.MessageList,
	destination string) ([]byte, error) {

	plain_text, err := proto.Marshal(message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	compressed_message_lists := [][]byte{plain_text}
	output_cipher, err := client.NewCipher(self.client_id,
		self.client_private_key, &self.server_private_key.PublicKey)
	if err != nil {
		return nil, err
	}

	packed_message_list := &crypto_proto.PackedMessageList{
		Compression: crypto_proto.PackedMessageList_UNCOMPRESSED,
		MessageList: compressed_message_lists,
		Nonce:       self.config_obj.Client.Nonce,
		Timestamp:   uint64(time.Now().UnixNano() / 1000),
	}

	serialized_packed_message_list, err := proto.Marshal(packed_message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	comms := output_cipher.ClientCommunication()

	// Each packet has a new IV.
	_, err = rand.Read(comms.PacketIv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	encrypted_serialized_packed_message_list, err := client.EncryptSymmetric(
		output_cipher.CipherProperties(),
		serialized_packed_message_list,
		comms.PacketIv)
	if err != nil {
		return nil, err

	}

	comms.Encrypted = encrypted_serialized_packed_message_list
	comms.FullHmac = client.CalcHMAC(comms, output_cipher.CipherProperties())

	result, err := proto.Marshal(comms)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return result, nil
}

func (self *TestSuite) TestEncDecClientToServer() {
	t := self.T()
	message_list := &crypto_proto.MessageList{}
	for i := 0; i < 5; i++ {
		message_list.Job = append(
			message_list.Job, &crypto_proto.VeloMessage{
				Name: "OMG it's a string"})
	}

	config_obj := config.GetDefaultConfig()
	cipher_text, err := self.client_manager.EncryptMessageList(
		message_list,
		crypto_proto.PackedMessageList_ZCOMPRESSION,
		config_obj.Client.PinnedServerName)
	assert.NoError(t, err)

	initial_c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)

	// Decrypt the same message 100 times.
	for i := 0; i < 100; i++ {
		message_info, err := self.server_manager.Decrypt(cipher_text)
		if err != nil {
			t.Fatal(err)
		}

		message_info.IterateJobs(context.Background(),
			func(ctx context.Context, item *crypto_proto.VeloMessage) {
				assert.Equal(t, item.Name, "OMG it's a string")
				assert.Equal(
					t, item.AuthState, crypto_proto.VeloMessage_AUTHENTICATED)
			})
	}

	// This should only do the RSA operation once since it should
	// hit the LRU cache each time.
	c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)
	assert.Equal(t, c-initial_c, float64(1))
}

func (self *TestSuite) TestEncryption() {
	t := self.T()
	plain_text := []byte("hello world")

	config_obj := config.GetDefaultConfig()
	initial_c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)
	for i := 0; i < 100; i++ {
		compressed, err := utils.Compress(plain_text)
		assert.NoError(t, err)

		cipher_text, err := self.client_manager.Encrypt(
			[][]byte{compressed},
			crypto_proto.PackedMessageList_ZCOMPRESSION,
			config_obj.Client.PinnedServerName)
		assert.NoError(t, err)

		result, err := self.server_manager.Decrypt(cipher_text)
		assert.NoError(t, err)

		assert.Equal(t, self.client_id, result.Source)
		assert.Equal(t, result.Authenticated, true)

		assert.Equal(t, result.RawCompressed[0], compressed)
	}

	// We should encrypt this only once since we cache the cipher in the output LRU.
	c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)
	assert.Equal(t, c-initial_c, float64(1))
}

func (self *TestSuite) TestClientIDFromPublicKey() {
	t := self.T()

	client_private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(self.config_obj.Writeback.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}

	client_id := crypto_utils.ClientIDFromPublicKey(&client_private_key.PublicKey)
	assert.True(t, strings.HasPrefix(client_id, "C."))
}

func TestMain(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
