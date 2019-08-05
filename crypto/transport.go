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
	"bytes"
	"compress/zlib"
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
	"io"
	"io/ioutil"
	"math/big"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
)

var (
	rsaSignCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsa_sign_op",
		Help: "Total number of rsa signatures.",
	})

	rsaEncryptCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsa_encrypt_op",
		Help: "Total number of rsa encryption ops.",
	})

	rsaDecryptCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsa_decrypt_op",
		Help: "Total number of rsa decryption ops.",
	})

	rsaVerifyCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsa_verify_op",
		Help: "Total number of rsa verify ops.",
	})
)

type _Cipher struct {
	key_size                  int
	cipher_properties         *crypto_proto.CipherProperties
	cipher_metadata           *crypto_proto.CipherMetadata
	encrypted_cipher          []byte
	encrypted_cipher_metadata []byte
}

func (self *_Cipher) Size() int {
	return 1
}

func _NewCipher(
	source string,
	private_key *rsa.PrivateKey,
	public_key *rsa.PublicKey) (*_Cipher, error) {

	result := &_Cipher{
		key_size: 128,
	}
	result.cipher_properties = &crypto_proto.CipherProperties{
		Name:       "aes_128_cbc",
		Key:        make([]byte, result.key_size/8),
		MetadataIv: make([]byte, result.key_size/8),
		HmacKey:    make([]byte, result.key_size/8),
		HmacType:   crypto_proto.CipherProperties_FULL_HMAC,
	}

	_, err := rand.Read(result.cipher_properties.Key)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = rand.Read(result.cipher_properties.MetadataIv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = rand.Read(result.cipher_properties.HmacKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result.cipher_metadata = &crypto_proto.CipherMetadata{
		Source: source,
	}

	serialized_cipher, err := proto.Marshal(result.cipher_properties)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	hashed := sha256.Sum256(serialized_cipher)
	rsaSignCounter.Inc()
	signature, err := rsa.SignPKCS1v15(
		rand.Reader, private_key, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	result.cipher_metadata.Signature = signature

	rsaEncryptCounter.Inc()
	encrypted_cipher, err := rsa.EncryptOAEP(
		sha1.New(), rand.Reader,
		public_key,
		serialized_cipher, []byte(""))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result.encrypted_cipher = encrypted_cipher

	serialized_cipher_metadata, err := proto.Marshal(result.cipher_metadata)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	encrypted_cipher_metadata, err := encryptSymmetric(
		result.cipher_properties,
		serialized_cipher_metadata,
		result.cipher_properties.MetadataIv)
	if err != nil {
		return nil, err
	}
	result.encrypted_cipher_metadata = encrypted_cipher_metadata

	return result, nil
}

type ICryptoManager interface {
	GetCSR() ([]byte, error)
	AddCertificate(certificate_pem []byte) (*string, error)
	/*	EncryptMessageList(
		message_list *crypto_proto.MessageList,
		destination string) ([]byte, error)
	*/Encrypt(plain_text []byte, destination string) ([]byte, error)
	Decrypt(cipher_text []byte) (*MessageInfo, error)
	//DecryptMessageList(cipher_text []byte) (*crypto_proto.MessageList, error)
}

type CryptoManager struct {
	config      *api_proto.Config
	private_key *rsa.PrivateKey

	source string

	public_key_resolver publicKeyResolver

	// Cache output cipher sessions for each destination. Sending
	// to the same destination will reuse the same cipher object
	// and therefore the same RSA keys.
	output_cipher_cache *cache.LRUCache

	// Cache cipher objects which have been verified.
	input_cipher_cache *cache.LRUCache

	caPool *x509.CertPool

	logger *logging.LogContext
}

// Clear all internal caches.
func (self *CryptoManager) Clear() {
	self.output_cipher_cache.Clear()
	self.input_cipher_cache.Clear()
	self.public_key_resolver.Clear()
}

func (self *CryptoManager) GetCSR() ([]byte, error) {
	subj := pkix.Name{
		CommonName: "aff4:/" + ClientIDFromPublicKey(
			&self.private_key.PublicKey),
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

// Adds the server certificate to the crypto manager.
func (self *CryptoManager) AddCertificate(certificate_pem []byte) (*string, error) {
	server_cert, err := ParseX509CertFromPemStr(certificate_pem)
	if err != nil {
		return nil, err
	}

	if server_cert.PublicKeyAlgorithm != x509.RSA {
		return nil, errors.New("Not RSA algorithm")
	}

	// Verify that the certificate is signed by the CA.
	opts := x509.VerifyOptions{
		Roots: self.caPool,
	}

	_, err = server_cert.Verify(opts)
	if err != nil {
		return nil, err
	}

	// Check that the server's serial number is larger than the
	// last one we saw. This prevents attackers from MITM old certs.
	last_serial_number := big.NewInt(int64(
		self.config.Writeback.LastServerSerialNumber))
	if last_serial_number.Cmp(server_cert.SerialNumber) == 1 {
		return nil, errors.New(
			fmt.Sprintf("Server serial number is too old. Should be %v",
				self.config.Writeback.LastServerSerialNumber))
	}

	// Server has advanced its serial number - record the new
	// number in our writeback state. Note- serial number can only
	// be advanced.

	// With the use of TLS I am not sure this code is needed. It
	// may also erroneously increment serial numbers then lock the
	// client out. It is disabled for now - we need to explictly
	// update the minimum server serial number from the server
	// when needed.

	// last_serial_number < server_cert.SerialNumber
	if false && last_serial_number.Cmp(server_cert.SerialNumber) == -1 {
		// Clear all our internal caches because we are now
		// re-keying.
		self.Clear()

		// Persist the number.
		self.config.Writeback.LastServerSerialNumber = uint64(
			server_cert.SerialNumber.Int64())
		err := config.UpdateWriteback(self.config)
		if err != nil {
			return nil, err
		}
		self.logger.Info(
			"Updated server serial number in "+
				"config file %v to %v",
			config.WritebackLocation(self.config),
			self.config.Writeback.LastServerSerialNumber)
	}

	err = self.public_key_resolver.SetPublicKey(
		server_cert.Subject.CommonName,
		server_cert.PublicKey.(*rsa.PublicKey))
	if err != nil {
		return nil, err
	}

	return &server_cert.Subject.CommonName, nil
}

func (self *CryptoManager) AddCertificateRequest(csr_pem []byte) (*string, error) {
	csr, err := parseX509CSRFromPemStr(csr_pem)
	if err != nil {
		return nil, err
	}

	if csr.PublicKeyAlgorithm != x509.RSA {
		return nil, errors.New("Not RSA algorithm")
	}

	common_name := strings.TrimPrefix(csr.Subject.CommonName, "aff4:/")
	public_key := csr.PublicKey.(*rsa.PublicKey)

	// CSRs are always generated by clients and therefore must
	// follow the rules about client id - make sure the client id
	// matches the public key.

	// NOTE: We do not actually sign the CSR at all - since the
	// client is free to generate its own private/public key pair
	// and just uses those to communicate with the server we just
	// store its public key so we can verify its
	// transmissions. The most important thing here is to verfiy
	// that the client id this packet claims to come from
	// corresponds with the public key this client presents. This
	// avoids the possibility of impersonation since the
	// public/private key pair is tied into the client id itself.
	if common_name != ClientIDFromPublicKey(public_key) {
		return nil, errors.New("Invalid CSR")
	}
	err = self.public_key_resolver.SetPublicKey(
		common_name, csr.PublicKey.(*rsa.PublicKey))
	if err != nil {
		return nil, err
	}
	return &csr.Subject.CommonName, nil
}

func NewCryptoManager(config_obj *api_proto.Config, source string, pem_str []byte) (
	*CryptoManager, error) {
	private_key, err := parseRsaPrivateKeyFromPemStr(pem_str)
	if err != nil {
		return nil, err
	}

	return &CryptoManager{
		config:              config_obj,
		private_key:         private_key,
		source:              source,
		public_key_resolver: NewInMemoryPublicKeyResolver(),
		output_cipher_cache: cache.NewLRUCache(1000),
		input_cipher_cache:  cache.NewLRUCache(1000),
		logger: logging.GetLogger(
			config_obj, &logging.ClientComponent),
	}, nil
}

func NewServerCryptoManager(config_obj *api_proto.Config) (*CryptoManager, error) {
	cert, err := ParseX509CertFromPemStr([]byte(config_obj.Frontend.Certificate))
	if err != nil {
		return nil, err
	}

	private_key, err := parseRsaPrivateKeyFromPemStr([]byte(
		config_obj.Frontend.PrivateKey))
	if err != nil {
		return nil, err
	}

	return &CryptoManager{
		config:              config_obj,
		private_key:         private_key,
		source:              cert.Subject.CommonName,
		public_key_resolver: NewServerPublicKeyResolver(config_obj),
		output_cipher_cache: cache.NewLRUCache(1000),
		input_cipher_cache:  cache.NewLRUCache(1000),
		logger: logging.GetLogger(config_obj,
			&logging.FrontendComponent),
	}, nil
}

func NewClientCryptoManager(config_obj *api_proto.Config, client_private_key_pem []byte) (
	*CryptoManager, error) {
	private_key, err := parseRsaPrivateKeyFromPemStr(client_private_key_pem)
	if err != nil {
		return nil, err
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	client_id := ClientIDFromPublicKey(&private_key.PublicKey)
	logger.Info("Starting Crypto for client %v", client_id)

	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	if !ok {
		return nil, errors.New("failed to parse CA certificate")
	}

	return &CryptoManager{
		config:              config_obj,
		private_key:         private_key,
		source:              client_id,
		public_key_resolver: NewInMemoryPublicKeyResolver(),
		output_cipher_cache: cache.NewLRUCache(1000),
		input_cipher_cache:  cache.NewLRUCache(1000),
		caPool:              roots,
		logger:              logger,
	}, nil
}

// Once a message is decoded the MessageInfo contains metadata about it.
type MessageInfo struct {
	Raw           []byte
	Authenticated bool
	Source        string
	RemoteAddr    string
}

/* Verify the HMAC protecting the cipher properties blob.

   The HMAC ensures that the cipher properties can not be modified.
*/
func (self *CryptoManager) calcHMAC(
	comms *crypto_proto.ClientCommunication,
	cipher *crypto_proto.CipherProperties) []byte {
	msg := comms.Encrypted
	msg = append(msg, comms.EncryptedCipher...)
	msg = append(msg, comms.EncryptedCipherMetadata...)
	msg = append(msg, comms.PacketIv...)

	temp := make([]byte, 4)
	binary.LittleEndian.PutUint32(temp, comms.ApiVersion)
	msg = append(msg, temp...)

	mac := hmac.New(sha1.New, cipher.HmacKey)
	mac.Write(msg)

	return mac.Sum(nil)
}

func encryptSymmetric(
	cipher_properties *crypto_proto.CipherProperties,
	plain_text []byte,
	iv []byte) ([]byte, error) {
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
		return nil, errors.WithStack(err)
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
		return nil, errors.WithStack(err)
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
	cipher_metadata *crypto_proto.CipherMetadata,
	serialized_cipher []byte,
	cipher_properties *crypto_proto.CipherProperties) (bool, error) {

	// Verify the cipher signature using the certificate known for
	// the sender.
	public_key, pres := self.public_key_resolver.GetPublicKey(cipher_metadata.Source)
	if !pres {
		// We dont know who we are talking to so we can not
		// trust them.
		return false, errors.New(
			fmt.Sprintf("No cert found for %s", cipher_metadata.Source))
	}

	hashed := sha256.Sum256(serialized_cipher)

	rsaVerifyCounter.Inc()
	err := rsa.VerifyPKCS1v15(public_key, crypto.SHA256, hashed[:],
		cipher_metadata.Signature)
	if err != nil {
		return false, errors.WithStack(err)
	}

	return true, nil
}

/* Decrypts an encrypted parcel. */
func (self *CryptoManager) Decrypt(cipher_text []byte) (*MessageInfo, error) {
	var err error
	// Parse the ClientCommunication protobuf.
	communications := &crypto_proto.ClientCommunication{}
	err = proto.Unmarshal(cipher_text, communications)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// An empty message is not an error but we cant figure out the
	// source.
	if len(communications.EncryptedCipher) == 0 {
		return &MessageInfo{}, nil
	}

	auth_state := false
	var cipher_properties *crypto_proto.CipherProperties

	v, ok := self.input_cipher_cache.Get(string(communications.EncryptedCipher))
	if ok {
		auth_state = true
		cipher_properties = v.(*_Cipher).cipher_properties

		// Check HMAC to save checking the RSA signature for
		// malformed packets.
		if !hmac.Equal(
			self.calcHMAC(communications, cipher_properties),
			communications.FullHmac) {
			return nil, errors.New("HMAC did not verify")
		}

	} else {
		// Decrypt the CipherProperties
		rsaDecryptCounter.Inc()
		serialized_cipher, err := rsa.DecryptOAEP(
			sha1.New(), rand.Reader,
			self.private_key,
			communications.EncryptedCipher,
			[]byte(""))
		if err != nil {
			return nil, errors.WithStack(err)
		}

		cipher_properties = &crypto_proto.CipherProperties{}
		err = proto.Unmarshal(serialized_cipher, cipher_properties)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// Check HMAC first to save checking the RSA signature for
		// malformed packets.
		if !hmac.Equal(
			self.calcHMAC(communications, cipher_properties),
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
			return nil, errors.WithStack(err)
		}

		// Verify the cipher metadata signature.
		auth_state, err = self.getAuthState(cipher_metadata,
			serialized_cipher,
			cipher_properties)

		// If we could verify the authentication state and it
		// was authenticated, we are now allowed to cache the
		// cipher in the input cache. The next packet from
		// this session will NOT be verified.
		if err == nil && auth_state {
			self.input_cipher_cache.Set(
				string(communications.EncryptedCipher),
				&_Cipher{cipher_properties: cipher_properties},
			)
		}

	}

	// Decrypt the cipher metadata.
	plain, err := decryptSymmetric(
		cipher_properties,
		communications.Encrypted,
		communications.PacketIv)
	if err != nil {
		return nil, err
	}

	// Unpack the message list.
	packed_message_list := &crypto_proto.PackedMessageList{}
	err = proto.Unmarshal(plain, packed_message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Check the nonce is correct.
	if packed_message_list.Nonce != self.config.Client.Nonce {
		return nil, errors.New(
			"Client Nonce is not valid - rejecting message.")
	}

	serialized_message_list := packed_message_list.MessageList
	if packed_message_list.Compression ==
		crypto_proto.PackedMessageList_ZCOMPRESSION {
		b := bytes.NewReader(serialized_message_list)
		z, err := zlib.NewReader(b)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer z.Close()
		p, err := ioutil.ReadAll(io.LimitReader(
			z, int64(self.config.Frontend.MaxUploadSize*2)))
		if err != nil {
			return nil, errors.WithStack(err)
		}

		serialized_message_list = p
	}

	return &MessageInfo{
		Raw:           serialized_message_list,
		Authenticated: auth_state,
		Source:        packed_message_list.Source,
	}, nil
}

// GRR usually encodes a MessageList protobuf inside the encrypted
// payload. This convenience method parses that type of payload after
// decrypting it.
func (self *CryptoManager) DecryptMessageList(cipher_text []byte) (
	*crypto_proto.MessageList, error) {
	message_info, err := self.Decrypt(cipher_text)
	if err != nil {
		return nil, err
	}

	result := &crypto_proto.MessageList{}
	err = proto.Unmarshal(message_info.Raw, result)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for _, message := range result.Job {
		if message_info.Authenticated {
			message.AuthState = crypto_proto.GrrMessage_AUTHENTICATED
		}
		message.Source = message_info.Source
	}

	return result, nil
}

func (self *CryptoManager) EncryptMessageList(
	message_list *crypto_proto.MessageList,
	destination string) ([]byte, error) {

	plain_text, err := proto.Marshal(message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cipher_text, err := self.Encrypt(plain_text, destination)
	return cipher_text, err
}

func (self *CryptoManager) Encrypt(
	plain_text []byte,
	destination string) (
	[]byte, error) {
	// The cipher is kept the same for all future communications
	// to enable the remote end to cache it - thereby saving RSA
	// operations for all messages in the session.
	var output_cipher *_Cipher

	output, ok := self.output_cipher_cache.Get(destination)
	if ok {
		output_cipher = output.(*_Cipher)
	} else {
		public_key, pres := self.public_key_resolver.GetPublicKey(destination)
		if !pres {
			return nil, errors.New(fmt.Sprintf(
				"No certificate found for destination %v",
				destination))
		}

		cipher, err := _NewCipher(self.source, self.private_key, public_key)
		if err != nil {
			return nil, err
		}

		self.output_cipher_cache.Set(destination, cipher)
		output_cipher = cipher
	}

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write([]byte(plain_text))
	w.Close()

	packed_message_list := &crypto_proto.PackedMessageList{
		Compression: crypto_proto.PackedMessageList_ZCOMPRESSION,
		MessageList: b.Bytes(),
		Source:      self.source,
		Nonce:       self.config.Client.Nonce,
		Timestamp:   uint64(time.Now().UnixNano() / 1000),
	}

	serialized_packed_message_list, err := proto.Marshal(packed_message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	comms := &crypto_proto.ClientCommunication{
		EncryptedCipher:         output_cipher.encrypted_cipher,
		EncryptedCipherMetadata: output_cipher.encrypted_cipher_metadata,
		PacketIv:                make([]byte, output_cipher.key_size/8),
		ApiVersion:              3,
	}

	// Each packet has a new IV.
	_, err = rand.Read(comms.PacketIv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	encrypted_serialized_packed_message_list, err := encryptSymmetric(
		output_cipher.cipher_properties,
		serialized_packed_message_list,
		comms.PacketIv)
	if err != nil {
		return nil, err
	}

	comms.Encrypted = encrypted_serialized_packed_message_list
	comms.FullHmac = self.calcHMAC(comms, output_cipher.cipher_properties)

	result, err := proto.Marshal(comms)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return result, nil
}
