package client

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"github.com/go-errors/errors"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vcrypto "www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type CryptoManager struct {
	config      *config_proto.Config
	private_key *rsa.PrivateKey

	Resolver  PublicKeyResolver
	client_id string

	// Cache output cipher sessions for each destination. Sending
	// to the same destination will reuse the same cipher object
	// and therefore the same RSA keys.
	cipher_lru *CipherLRU

	// A cache of unauthenticated cipher objects. They will expire
	// from the cache within a few minutes but this avoids us having
	// to decrypt them during enrollment when all messages are
	// unauthenticated.
	unauthenticated_lru *ttlcache.Cache

	caPool *x509.CertPool

	logger *logging.LogContext

	limiter *rate.Limiter
}

// Clear all internal caches.
func (self *CryptoManager) Clear() {
	self.cipher_lru.Clear()
	self.Resolver.Clear()
	self.unauthenticated_lru.Flush()
}

func (self *CryptoManager) ClientId() string {
	return self.client_id
}

// Delete all caches related to the subject name (client id).
func (self *CryptoManager) DeleteSubject(client_id string) {
	self.cipher_lru.DeleteCipher(client_id)
	self.Resolver.DeleteSubject(client_id)
}

func (self *CryptoManager) GetCSR() ([]byte, error) {
	subj := pkix.Name{
		CommonName: crypto_utils.ClientIDFromPublicKey(&self.private_key.PublicKey),
	}

	template := x509.CertificateRequest{
		Subject:            subj,
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	csrBytes, _ := x509.CreateCertificateRequest(
		rand.Reader, &template, self.private_key)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes}), nil
}

func NewCryptoManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	private_key_pem []byte,
	public_key_resolver PublicKeyResolver,
	logger *logging.LogContext) (
	*CryptoManager, error) {

	private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(private_key_pem)
	if err != nil {
		return nil, err
	}

	limit_rate := int64(100)
	if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources.EnrollmentsPerSecond > 0 {
		limit_rate = config_obj.Frontend.Resources.EnrollmentsPerSecond
	}

	result := &CryptoManager{
		config:              config_obj,
		private_key:         private_key,
		client_id:           client_id,
		Resolver:            public_key_resolver,
		cipher_lru:          NewCipherLRU(config_obj.Frontend.Resources.ExpectedClients),
		unauthenticated_lru: ttlcache.NewCache(),
		logger:              logging.GetLogger(config_obj, &logging.ClientComponent),
		limiter:             rate.NewLimiter(rate.Limit(limit_rate), 100),
	}

	_ = result.unauthenticated_lru.SetTTL(time.Second * 60)
	result.unauthenticated_lru.SkipTTLExtensionOnHit(true)

	go func() {
		<-ctx.Done()
		result.unauthenticated_lru.Close()
	}()

	return result, nil
}

/*
Verify the HMAC protecting the cipher properties blob.

The HMAC ensures that the cipher properties can not be modified.
*/
func CalcHMAC(
	comms *crypto_proto.ClientCommunication,
	cipher *crypto_proto.CipherProperties) []byte {
	msg := make([]byte, 0,
		len(comms.Encrypted)+len(comms.EncryptedCipher)+
			len(comms.EncryptedCipherMetadata)+len(comms.PacketIv)+64)
	msg = append(msg, comms.Encrypted...)
	msg = append(msg, comms.EncryptedCipher...)
	msg = append(msg, comms.EncryptedCipherMetadata...)
	msg = append(msg, comms.PacketIv...)

	temp := make([]byte, 4)
	binary.LittleEndian.PutUint32(temp, comms.ApiVersion)
	msg = append(msg, temp...)

	mac := hmac.New(sha1.New, cipher.HmacKey)
	_, _ = mac.Write(msg)
	return mac.Sum(nil)
}

func EncryptSymmetric(
	cipher_properties *crypto_proto.CipherProperties,
	plain_text []byte, iv []byte) ([]byte, error) {
	if len(cipher_properties.Key) != 16 {
		return nil, errors.New("Incorrect key length provided.")
	}

	// Add padding. See
	// https://tools.ietf.org/html/rfc5246#section-6.2.3.2 for
	// details.
	padding := aes.BlockSize - (len(plain_text) % aes.BlockSize)
	for i := 0; i < padding; i++ {
		plain_text = append(plain_text, byte(padding))
	}

	base_crypter, err := aes.NewCipher(cipher_properties.Key)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	mode := cipher.NewCBCEncrypter(base_crypter, iv)
	cipher_text := make([]byte, len(plain_text))
	mode.CryptBlocks(cipher_text, plain_text)

	return cipher_text, nil
}

func decryptSymmetric(
	cipher_properties *crypto_proto.CipherProperties,
	cipher_text []byte,
	iv []byte) ([]byte, error) {
	if len(cipher_properties.Key) != 16 {
		return nil, errors.New("Incorrect key length provided.")
	}

	if len(cipher_text)%aes.BlockSize != 0 {
		return nil, errors.New("Cipher test is not whole number of blocks")
	}

	base_crypter, err := aes.NewCipher(cipher_properties.Key)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	mode := cipher.NewCBCDecrypter(base_crypter, iv)
	plain_text := make([]byte, len(cipher_text))
	mode.CryptBlocks(plain_text, cipher_text)

	// Strip the padding. See
	// https://tools.ietf.org/html/rfc5246#section-6.2.3.2 for
	// details.
	padding := int(plain_text[len(plain_text)-1])
	for i := len(plain_text) - padding; i < len(plain_text); i++ {
		if int(plain_text[i]) != padding {
			return nil, errors.New("Padding error")
		}
	}

	return plain_text[:len(plain_text)-padding], nil
}

func (self *CryptoManager) getAuthState(
	config_obj *config_proto.Config,
	cipher_metadata *crypto_proto.CipherMetadata,
	serialized_cipher []byte,
	cipher_properties *crypto_proto.CipherProperties) (bool, error) {

	// Verify the cipher signature using the certificate known for
	// the sender.
	client_id := utils.ClientIdFromSource(cipher_metadata.Source)
	public_key, pres := self.Resolver.GetPublicKey(
		config_obj, cipher_metadata.Source)
	if !pres {
		// Try to extract an org id from the source in case the public
		// key was added without one.
		public_key, pres = self.Resolver.GetPublicKey(config_obj, client_id)
		if !pres {
			// We dont know who we are talking to so we can not trust
			// them.
			return false,
				fmt.Errorf("No cert found for %s", cipher_metadata.Source)
		}
	}

	hashed := sha256.Sum256(serialized_cipher)

	RsaVerifyCounter.Inc()
	err := rsa.VerifyPKCS1v15(public_key, crypto.SHA256, hashed[:],
		cipher_metadata.Signature)
	if err != nil {
		return false, errors.Wrap(err, 0)
	}

	return true, nil
}

func (self *CryptoManager) getCachedCipher(encrypted_cipher []byte) (*_Cipher, bool) {
	cipher, ok := self.cipher_lru.GetByInboundCipher(encrypted_cipher)
	if ok {
		return cipher, ok
	}

	cipher_any, err := self.unauthenticated_lru.Get(string(encrypted_cipher))
	if err == nil {
		cipher, ok := cipher_any.(*_Cipher)
		if ok {
			return cipher, true
		}
	}
	return nil, false
}

/* Decrypts an encrypted parcel and produces a MessageInfo. */
func (self *CryptoManager) Decrypt(
	ctx context.Context,
	cipher_text []byte) (*vcrypto.MessageInfo, error) {
	var err error
	// Parse the ClientCommunication protobuf.
	communications := &crypto_proto.ClientCommunication{}
	err = proto.Unmarshal(cipher_text, communications)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	// An empty message is not an error but we can't figure out the
	// source.
	if len(communications.EncryptedCipher) == 0 {
		return &vcrypto.MessageInfo{}, nil
	}

	cipher, ok := self.getCachedCipher(communications.EncryptedCipher)
	if ok {
		// Check HMAC to save checking the RSA signature for
		// malformed packets.
		if !hmac.Equal(
			CalcHMAC(communications, cipher.cipher_properties),
			communications.FullHmac) {
			return nil, errors.New("HMAC did not verify")
		}

		msg_info, _, err := self.extractMessageInfo(
			cipher.cipher_properties, communications)
		if err != nil {
			return nil, err
		}

		// Cipher was cached so we trust it
		msg_info.Authenticated = cipher.authenticated
		msg_info.Source = cipher.cipher_metadata.Source

		return msg_info, nil
	}

	if self.limiter != nil {
		err = self.limiter.Wait(ctx)
		if err != nil {
			return nil, err
		}
	}

	// Decrypt the CipherProperties
	RsaDecryptCounter.Inc()
	serialized_cipher, err := rsa.DecryptOAEP(
		sha1.New(), rand.Reader,
		self.private_key,
		communications.EncryptedCipher,
		[]byte(""))
	if err != nil {
		return nil, err
	}

	cipher_properties := &crypto_proto.CipherProperties{}
	err = proto.Unmarshal(serialized_cipher, cipher_properties)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	// Check HMAC first to save checking the RSA signature for
	// malformed packets.
	if !hmac.Equal(
		CalcHMAC(communications, cipher_properties),
		communications.FullHmac) {
		return nil, errors.New("HMAC did not verify")
	}

	// Extract the serialized CipherMetadata.
	serialized_metadata, err := decryptSymmetric(
		cipher_properties, communications.EncryptedCipherMetadata,
		cipher_properties.MetadataIv)
	if err != nil {
		return nil, err
	}

	cipher_metadata := &crypto_proto.CipherMetadata{}
	err = proto.Unmarshal(serialized_metadata, cipher_metadata)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	msg_info, org_config_obj, err := self.extractMessageInfo(
		cipher_properties, communications)
	if err != nil {
		return nil, err
	}

	// Verify the cipher metadata signature.
	cipher_metadata.Source = utils.ClientIdFromSourceAndOrg(
		cipher_metadata.Source, msg_info.OrgId)

	msg_info.Authenticated, err = self.getAuthState(
		org_config_obj, cipher_metadata, serialized_cipher, cipher_properties)

	// Make sure the message source is set from the cipher_metadata
	// overriding the internal Source. The source is cryptographically
	// verified by the encryption of the outer envelop.
	msg_info.Source = cipher_metadata.Source

	// If we could verify the authentication state and it
	// was authenticated, we are now allowed to cache the
	// cipher in the input cache. The next packet from
	// this session will NOT be verified.
	if err == nil && msg_info.Authenticated {
		self.cipher_lru.Set(
			msg_info.Source,
			&_Cipher{
				cipher_metadata:   cipher_metadata,
				encrypted_cipher:  communications.EncryptedCipher,
				cipher_properties: cipher_properties,
				authenticated:     msg_info.Authenticated,
			},
			nil, /* outbound_cipher */
		)
		return msg_info, nil
	}

	_ = self.unauthenticated_lru.Set(
		msg_info.Source,
		&_Cipher{
			cipher_metadata:   cipher_metadata,
			encrypted_cipher:  communications.EncryptedCipher,
			cipher_properties: cipher_properties,
			authenticated:     false,
		})
	return msg_info, nil
}

// Decrypt the message from the communications using the cipher
// properties.
func (self *CryptoManager) extractMessageInfo(
	cipher_properties *crypto_proto.CipherProperties,
	communications *crypto_proto.ClientCommunication) (
	*vcrypto.MessageInfo, *config_proto.Config, error) {

	// Decrypt the cipher metadata.
	plain, err := decryptSymmetric(
		cipher_properties,
		communications.Encrypted,
		communications.PacketIv)
	if err != nil {
		return nil, nil, err
	}

	// Unpack the message list.
	packed_message_list := &crypto_proto.PackedMessageList{}
	err = proto.Unmarshal(plain, packed_message_list)
	if err != nil {
		return nil, nil, errors.Wrap(err, 0)
	}

	// Get the org id from the nonce
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, nil, err
	}

	org_id, err := org_manager.OrgIdByNonce(packed_message_list.Nonce)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"Client Nonce %v is not valid - rejecting message.",
			packed_message_list.Nonce)
	}

	org_config_obj, err := org_manager.GetOrgConfig(org_id)
	if err != nil {
		return nil, nil, err
	}

	return &vcrypto.MessageInfo{
		Version: communications.ApiVersion,
		// Hold onto the compressed MessageList buffers.
		RawCompressed: packed_message_list.MessageList,
		Compression:   packed_message_list.Compression,
		OrgId:         org_id,
	}, org_config_obj, nil
}

// Serialize, compress and encrypt a single message list proto. NOTE:
// When the client sends back bulk data, they pack messages into the
// MessageList proto and call this function. Since they dont know in
// advance how large the compressed size is going to be, they need to
// send multiple MessageList protos, each compressed separatly until
// there are enough to send.
func (self *CryptoManager) EncryptMessageList(
	message_list *crypto_proto.MessageList,
	compression crypto_proto.PackedMessageList_CompressionType,
	nonce, destination string) ([]byte, error) {

	plain_text, err := proto.Marshal(message_list)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	if compression == crypto_proto.PackedMessageList_ZCOMPRESSION {
		plain_text, err = utils.Compress(plain_text)
		if err != nil {
			return nil, errors.Wrap(err, 0)
		}
	}

	cipher_text, err := self.Encrypt(
		[][]byte{plain_text},
		compression,
		nonce, destination)
	return cipher_text, err
}

// Encrypt a message for sending to the other end.
func (self *CryptoManager) Encrypt(
	compressed_message_lists [][]byte,
	compression crypto_proto.PackedMessageList_CompressionType,
	nonce string,
	destination string) (
	[]byte, error) {

	// Get the config that relates to the destination.
	org_id := utils.OrgIdFromClientId(destination)
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(org_id)
	if err != nil {
		return nil, err
	}

	// The cipher is kept the same for all future communications
	// to enable the remote end to cache it - thereby saving RSA
	// operations for all messages in the session.
	output_cipher, ok := self.cipher_lru.GetOutboundCipher(destination)
	if !ok {
		// Build a new cipher
		public_key, pres := self.Resolver.GetPublicKey(org_config_obj, destination)
		if !pres {
			return nil, fmt.Errorf(
				"No certificate found for destination %v",
				destination)
		}

		cipher, err := NewCipher(self.client_id, self.private_key, public_key)
		if err != nil {
			return nil, err
		}

		self.cipher_lru.Set(destination, nil, cipher)
		output_cipher = cipher
	}

	packed_message_list := &crypto_proto.PackedMessageList{
		// We always compress the data.
		Compression: compression,
		MessageList: compressed_message_lists,
		Nonce:       nonce,
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

	encrypted_serialized_packed_message_list, err := EncryptSymmetric(
		output_cipher.cipher_properties,
		serialized_packed_message_list,
		comms.PacketIv)
	if err != nil {
		return nil, err

	}

	comms.Encrypted = encrypted_serialized_packed_message_list
	comms.FullHmac = CalcHMAC(comms, output_cipher.cipher_properties)

	result, err := proto.Marshal(comms)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	return result, nil
}
