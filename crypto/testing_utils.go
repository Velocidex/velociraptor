package crypto

import (
	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type NullCryptoManager struct{}

func (self *NullCryptoManager) GetCSR() ([]byte, error) {
	return []byte{}, nil
}
func (self *NullCryptoManager) AddCertificate(certificate_pem []byte) (
	*string, error) {
	return nil, nil
}

func (self *NullCryptoManager) EncryptMessageList(
	message_list *crypto_proto.MessageList,
	destination string) ([]byte, error) {
	plain_text, err := proto.Marshal(message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cipher_text, err := self.Encrypt(
		[][]byte{utils.Compress(plain_text)},
		crypto_proto.PackedMessageList_ZCOMPRESSION,
		destination)
	return cipher_text, err
}

func (self *NullCryptoManager) Encrypt(
	compressed_message_lists [][]byte,
	compression crypto_proto.PackedMessageList_CompressionType,
	destination string) (
	[]byte, error) {
	packed_message_list := &crypto_proto.PackedMessageList{
		MessageList: compressed_message_lists,
	}

	serialized_packed_message_list, err := proto.Marshal(packed_message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return serialized_packed_message_list, nil
}

func (self *NullCryptoManager) Decrypt(cipher_text []byte) (
	*MessageInfo, error) {

	packed_message_list := &crypto_proto.PackedMessageList{}
	err := proto.Unmarshal(cipher_text, packed_message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &MessageInfo{
		RawCompressed: packed_message_list.MessageList,
		Authenticated: true,
		Source:        "C.123456",
		Compression:   crypto_proto.PackedMessageList_ZCOMPRESSION,
	}, nil
}
