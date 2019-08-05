package crypto

import (
	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
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

	return plain_text, nil
}

func (self *NullCryptoManager) Encrypt(plain_text []byte, destination string) (
	[]byte, error) {
	return plain_text, nil
}

func (self *NullCryptoManager) Decrypt(cipher_text []byte) (
	*MessageInfo, error) {
	return &MessageInfo{
		Raw:           cipher_text,
		Authenticated: true,
		Source:        "C.123456",
	}, nil
}

func (self *NullCryptoManager) DecryptMessageList(cipher_text []byte) (
	*crypto_proto.MessageList, error) {
	result := &crypto_proto.MessageList{}
	err := proto.Unmarshal(cipher_text, result)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return result, nil
}
