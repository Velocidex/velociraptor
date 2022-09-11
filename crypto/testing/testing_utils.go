package testing

import (
	errors "github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/utils"
)

type NullCryptoManager struct{}

func (self *NullCryptoManager) GetCSR() ([]byte, error) {
	return []byte{}, nil
}
func (self *NullCryptoManager) AddCertificate(
	config_obj *config_proto.Config,
	certificate_pem []byte) (
	string, error) {

	server_cert, err := crypto_utils.ParseX509CertFromPemStr(certificate_pem)
	if err != nil {
		return "", err
	}

	return crypto_utils.GetSubjectName(server_cert), nil
}

func (self *NullCryptoManager) EncryptMessageList(
	message_list *crypto_proto.MessageList,
	destination string) ([]byte, error) {
	plain_text, err := proto.Marshal(message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	compressed, err := utils.Compress(plain_text)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cipher_text, err := self.Encrypt(
		[][]byte{compressed},
		crypto_proto.PackedMessageList_ZCOMPRESSION,
		"", destination)
	return cipher_text, err
}

func (self *NullCryptoManager) Encrypt(
	compressed_message_lists [][]byte,
	compression crypto_proto.PackedMessageList_CompressionType,
	nonce, destination string) (
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
	*crypto.MessageInfo, error) {

	packed_message_list := &crypto_proto.PackedMessageList{}
	err := proto.Unmarshal(cipher_text, packed_message_list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &crypto.MessageInfo{
		RawCompressed: packed_message_list.MessageList,
		Authenticated: true,
		Source:        "C.123456",
		Compression:   crypto_proto.PackedMessageList_ZCOMPRESSION,
	}, nil
}
