/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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

	errors "github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_server "www.velocidex.com/golang/velociraptor/crypto/server"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TestSuite struct {
	test_utils.TestSuite
	server_manager *crypto_server.ServerCryptoManager
	client_manager *crypto_client.ClientCryptoManager

	client_private_key *rsa.PrivateKey
	server_private_key *rsa.PrivateKey
	client_id          string
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services = &config_proto.ServerServicesConfig{
		IndexServer:    true,
		ClientInfo:     true,
		JournalService: true,
	}
	self.ConfigObj.Client.WritebackLinux = ""
	self.ConfigObj.Client.WritebackWindows = ""

	self.TestSuite.SetupTest()

	key, err := crypto_utils.GeneratePrivateKey()
	assert.NoError(self.T(), err)

	self.ConfigObj.Writeback.PrivateKey = string(key)

	// Configure the client manager.
	self.client_manager, err = crypto_client.NewClientCryptoManager(self.Ctx,
		self.ConfigObj, []byte(self.ConfigObj.Writeback.PrivateKey))
	require.NoError(self.T(), err)

	self.client_private_key, err = crypto_utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(self.ConfigObj.Writeback.PrivateKey))
	require.NoError(self.T(), err)

	self.server_private_key, err = crypto_utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(self.ConfigObj.Frontend.PrivateKey))
	require.NoError(self.T(), err)

	_, err = self.client_manager.AddCertificate(
		self.ConfigObj,
		[]byte(self.ConfigObj.Frontend.Certificate))
	require.NoError(self.T(), err)

	private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(key)
	assert.NoError(self.T(), err)

	self.client_id = crypto_utils.ClientIDFromPublicKey(&private_key.PublicKey)

	// Configure the server manager.
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	self.server_manager, err = crypto_server.NewServerCryptoManager(
		ctx, self.ConfigObj, wg)
	require.NoError(self.T(), err)

	// Install an in memory public key resolver.
	self.server_manager.Resolver = crypto_client.NewInMemoryPublicKeyResolver()
	self.server_manager.Resolver.SetPublicKey(
		self.ConfigObj, self.client_id, &private_key.PublicKey)
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
		self.ConfigObj.Client.Nonce,
		self.client_id)
	assert.NoError(t, err)

	initial_c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)

	// Decrypt the same message 100 times.
	for i := 0; i < 100; i++ {
		message_info, err := self.client_manager.Decrypt(self.Ctx, cipher_text)
		if err != nil {
			t.Fatal(err)
		}
		message_info.IterateJobs(context.Background(), self.ConfigObj,
			func(ctx context.Context, item *crypto_proto.VeloMessage) error {
				assert.Equal(t, item.Name, "OMG it's a string")
				assert.Equal(t, item.AuthState, crypto_proto.VeloMessage_AUTHENTICATED)
				return nil
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

	cipher_text, err := self._EncryptMessageListWithSpoofedPackedMessage(
		message_list, utils.GetSuperuserName(self.ConfigObj))
	assert.NoError(t, err)

	message_info, err := self.server_manager.Decrypt(self.Ctx, cipher_text)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, message_info.Source, self.client_id)
	err = message_info.IterateJobs(context.Background(), self.ConfigObj,
		func(ctx context.Context, msg *crypto_proto.VeloMessage) error {
			// Make sure the spoofed source is ignored, and the
			// correct source is relayed in the VeloMessage.
			assert.Equal(t, msg.Source, self.client_id)
			return nil
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
		return nil, errors.Wrap(err, 0)
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
		Nonce:       self.ConfigObj.Client.Nonce,
		Timestamp:   uint64(time.Now().UnixNano() / 1000),
	}

	serialized_packed_message_list, err := proto.Marshal(packed_message_list)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	comms := output_cipher.ClientCommunication()

	// Each packet has a new IV.
	_, err = rand.Read(comms.PacketIv)
	if err != nil {
		return nil, errors.Wrap(err, 0)
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
		return nil, errors.Wrap(err, 0)
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

	nonce := self.ConfigObj.Client.Nonce

	cipher_text, err := self.client_manager.EncryptMessageList(
		message_list,
		crypto_proto.PackedMessageList_ZCOMPRESSION,
		nonce, utils.GetSuperuserName(self.ConfigObj))
	assert.NoError(t, err)

	initial_c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)

	// Decrypt the same message 100 times.
	for i := 0; i < 100; i++ {
		message_info, err := self.server_manager.Decrypt(self.Ctx, cipher_text)
		if err != nil {
			t.Fatal(err)
		}

		message_info.IterateJobs(context.Background(), self.ConfigObj,
			func(ctx context.Context, item *crypto_proto.VeloMessage) error {
				assert.Equal(t, item.Name, "OMG it's a string")
				assert.Equal(
					t, item.AuthState, crypto_proto.VeloMessage_AUTHENTICATED)
				return nil
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

	initial_c := testutil.ToFloat64(crypto_client.RsaDecryptCounter)
	for i := 0; i < 100; i++ {
		compressed, err := utils.Compress(plain_text)
		assert.NoError(t, err)

		cipher_text, err := self.client_manager.Encrypt(
			[][]byte{compressed},
			crypto_proto.PackedMessageList_ZCOMPRESSION,
			self.ConfigObj.Client.Nonce,
			utils.GetSuperuserName(self.ConfigObj))
		assert.NoError(t, err)

		result, err := self.server_manager.Decrypt(self.Ctx, cipher_text)
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
		[]byte(self.ConfigObj.Writeback.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}

	client_id := crypto_utils.ClientIDFromPublicKey(&client_private_key.PublicKey)
	assert.True(t, strings.HasPrefix(client_id, "C."))
}

func TestMain(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
