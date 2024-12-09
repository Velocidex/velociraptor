package storage

import (
	"context"
	"errors"
	"io"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/crypto/server"
	crypto_server "www.velocidex.com/golang/velociraptor/crypto/server"
)

type ReaderAtCloser interface {
	io.ReaderAt
	Close() error
}

type CryptoFileReader struct {
	config_obj *config_proto.Config

	fd             ReaderAtCloser
	header         *Header
	crypto_manager *crypto_server.ServerCryptoManager
	client_id      string
}

func (self *CryptoFileReader) Close() error {
	return self.fd.Close()
}

func (self *CryptoFileReader) Parse(ctx context.Context) chan *crypto_proto.VeloMessage {
	output_chan := make(chan *crypto_proto.VeloMessage)

	go func() {
		defer close(output_chan)

		offset := self.header.FirstMessage
		for {
			next, err := self.readPacket(ctx, offset, output_chan)
			if err == io.EOF {
				return
			}

			if err != nil {
				return
			}
			offset = next
		}
	}()

	return output_chan
}

func (self *CryptoFileReader) readPacket(
	ctx context.Context,
	offset uint64,
	output_chan chan *crypto_proto.VeloMessage) (next uint64, err error) {

	// Read the message at point
	message := &Message{}
	err = message.ReadAt(self.fd, offset)
	if err != nil {
		return 0, err
	}

	switch message.Type {
	case PUB_KEY_TYPE:
		buf := make([]byte, message.Length)
		_, err := self.fd.ReadAt(buf, int64(message.Start))
		// Should verify the cert belongs to us
		return message.Next, err

	case CLIENT_CSR:
		buf := make([]byte, message.Length)
		n, err := self.fd.ReadAt(buf, int64(message.Start))
		if err != nil {
			return 0, err
		}

		self.client_id, err = self.crypto_manager.AddCertificateRequest(
			self.config_obj, buf[:n])
		if err != nil {
			return 0, err
		}
		return message.Next, nil

	case PACKET_TYPE:
		buf := make([]byte, message.Length)
		n, err := self.fd.ReadAt(buf, int64(message.Start))
		if err != nil {
			return 0, err
		}

		message_info, err := self.crypto_manager.Decrypt(ctx, buf[:n])
		if err != nil {
			return 0, err
		}

		// Emit all the messages in this packet
		err = message_info.IterateJobs(ctx, self.config_obj,
			func(ctx context.Context, msg *crypto_proto.VeloMessage) error {
				select {
				case <-ctx.Done():
					return nil
				case output_chan <- msg:
				}
				return nil
			})
		if err != nil {
			return 0, err
		}

		return message.Next, nil
	}

	// Skip the object
	return message.Next, nil
}

func NewCryptoFileReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	fd ReaderAtCloser) (*CryptoFileReader, error) {

	result := &CryptoFileReader{
		config_obj: config_obj,
		fd:         fd,
		header:     &Header{},
	}

	// Try to read the header to see if there is an existing file
	err := result.header.Read(fd)
	if err != nil {
		return nil, err
	}

	// Check the magic
	if result.header.Magic != MAGIC {
		return nil, errors.New("Invalid file magic")
	}

	if config_obj.Frontend == nil || config_obj.Frontend.Certificate == "" {
		return nil, errors.New("Reading Crypto Containers can only happen on the server")
	}

	var wg sync.WaitGroup
	result.crypto_manager, err = server.NewServerCryptoManager(ctx, config_obj, &wg)
	return result, err
}
