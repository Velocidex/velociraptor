package storage

import (
	"context"
	"errors"
	"os"
	"sync"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/crypto/utils"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/services/writeback"
)

type KeepPolicy bool

var (
	MAGIC        = [8]byte{'v', 'e', 'l', 'o', 0xf0, 0x0d, 0x00, 0x01}
	PUB_KEY_TYPE = [8]byte{'p', 'u', 'b', 'k', 'e', 'y', 0, 0}
	CLIENT_CSR   = [8]byte{'c', 's', 'r', 0, 0, 0, 0, 0}
	PACKET_TYPE  = [8]byte{'p', 'a', 'c', 'k', 'e', 't', 0, 0}

	KEEP_ON_ERROR = KeepPolicy(true)
)

type CryptoFileWriter struct {
	mu sync.Mutex

	ctx        context.Context
	config_obj *config_proto.Config
	fd         *os.File
	header     *Header
	messages   []*crypto_proto.VeloMessage

	crypto_manager *crypto_client.ClientCryptoManager

	// The PEM of the server
	server_pem  []byte
	server_name string

	written_headers bool

	max_size uint64
}

func (self *CryptoFileWriter) Close() error {
	defer self.fd.Close()

	return self.Flush(!KEEP_ON_ERROR)
}

func (self *CryptoFileWriter) serverPem() ([]byte, error) {
	server_pem := self.server_pem
	if len(server_pem) == 0 {
		server_pem = currentServerPEM
	}

	if len(server_pem) == 0 {
		return nil, errors.New("Server PEM not initialized yet")
	}

	self.server_pem = server_pem
	server_cert, err := utils.ParseX509CertFromPemStr(server_pem)
	if err != nil {
		return nil, err
	}

	self.server_name = crypto_utils.GetSubjectName(server_cert)

	return server_pem, nil
}

// Lazy creation of the crypto manager. We can not fully initialize
// the crypto manager until we have contacted the server and fetched
// its certificate. This code delays use of the crypto manager until
// it becomes available.
func (self *CryptoFileWriter) cryptoManager(ctx context.Context) (
	*crypto_client.ClientCryptoManager, error) {

	server_pem, err := self.serverPem()
	if err != nil {
		return nil, err
	}

	// Get the cached crypto manager
	if self.crypto_manager != nil {
		return self.crypto_manager, nil
	}

	// Bootstrap the crypto manager
	writeback_service := writeback.GetWritebackService()
	writeback, err := writeback_service.GetWriteback(self.config_obj)
	if err != nil {
		return nil, err
	}

	crypto_manager, err := crypto_client.NewClientCryptoManager(ctx,
		self.config_obj, []byte(writeback.PrivateKey))
	if err != nil {
		return nil, err
	}

	_, err = crypto_manager.AddCertificate(self.config_obj, server_pem)
	if err != nil {
		return nil, err
	}

	self.crypto_manager = crypto_manager
	return crypto_manager, nil
}

func (self *CryptoFileWriter) AddMessage(m *crypto_proto.VeloMessage) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.messages = append(self.messages, m)
}

func (self *CryptoFileWriter) Flush(keep_on_error KeepPolicy) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Check if we need to truncate the file. TODO: Think of a more
	// reasonable way to rotate the data in the file instead of justt
	// truncating it.
	if self.max_size > 0 && self.header.Next > self.max_size {
		err := self.fd.Truncate(0)
		if err != nil {
			return err
		}

		self.header.FirstMessage = 50
		self.header.Next = 50
		err = self.header.Write(self.fd)
		if err != nil {
			return err
		}
	}

	// Nothing to do here.
	if len(self.messages) == 0 {
		return nil
	}

	if !keep_on_error {
		defer func() {
			self.messages = nil
		}()
	}

	err := self.writeCerts()
	if err != nil {
		return err
	}

	message_list := &crypto_proto.MessageList{
		Job: self.messages,
	}

	serialized_msg, err := proto.Marshal(message_list)
	if err != nil {
		return err
	}

	nonce := self.config_obj.Client.Nonce

	manager, err := self.cryptoManager(self.ctx)
	if err != nil {
		return err
	}

	cipher_text, err := manager.Encrypt(
		[][]byte{serialized_msg}, crypto_proto.PackedMessageList_UNCOMPRESSED,
		nonce, self.server_name)
	if err != nil {
		return err
	}

	packet := &Packet{Data: cipher_text}
	err = packet.WriteAt(self.fd, int64(self.header.Next))
	if err != nil {
		return err
	}

	self.header.Next = packet.Next

	err = self.header.Write(self.fd)
	if err != nil {
		return err
	}

	self.messages = nil

	return nil
}

func (self *CryptoFileWriter) writeCerts() error {
	if self.written_headers {
		return nil
	}

	server_pem := self.server_pem
	if len(server_pem) == 0 {
		server_pem = currentServerPEM
	}

	if len(server_pem) == 0 {
		return errors.New("Server PEM not initialized")
	}

	// Add the pem to the file to rekey the reader. We will write it
	// at the next available position as indicated by the header.
	pub_key := PublicKey{Pem: server_pem}
	err := pub_key.WriteAt(self.fd, int64(self.header.Next))
	if err != nil {
		return err
	}

	self.header.Next = pub_key.Next

	manager, err := self.cryptoManager(self.ctx)
	if err != nil {
		return err
	}

	csr_pem, err := manager.GetCSR()
	if err != nil {
		return err
	}

	csr := &ClientCSR{Pem: csr_pem}
	err = csr.WriteAt(self.fd, int64(self.header.Next))
	if err != nil {
		return err
	}

	self.header.Next = csr.Next

	err = self.header.Write(self.fd)
	if err != nil {
		return err
	}

	self.written_headers = true
	return nil
}

func NewCryptoFileWriter(
	ctx context.Context,
	config_obj *config_proto.Config,
	max_size uint64,
	filename string) (*CryptoFileWriter, error) {

	if config_obj.Client == nil {
		return nil, errors.New("Crypto files require a valid Client config")
	}

	// Open file as without truncate (can be append)
	fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}

	stat, err := fd.Stat()
	if err != nil {
		fd.Close()
		return nil, err
	}

	// It is a symlink - we dont support those here!
	if stat.Mode()&os.ModeSymlink != 0 {
		fd.Close()
		return nil, errors.New("Symlink not supported")
	}

	result := &CryptoFileWriter{
		config_obj: config_obj,
		fd:         fd,
		ctx:        ctx,
		header:     &Header{},
		max_size:   max_size,
	}

	// Try to read the header to see if there is an existing file
	err = result.header.Read(fd)
	if err != nil {
		// Nope - truncate to 0 and start again
		err := fd.Truncate(0)
		if err != nil {
			return nil, err
		}

		result.header.FirstMessage = 50
		result.header.Next = 50
		return result, result.header.Write(fd)
	}

	// Check the magic
	if result.header.Magic != MAGIC {
		return nil, errors.New("Invalid file magic")
	}

	// Continue writing from where we left off.
	return result, nil
}
