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
package crypto

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type ICryptoManager interface {
	GetCSR() ([]byte, error)
	Encrypt(compressed_message_lists [][]byte,
		compression crypto_proto.PackedMessageList_CompressionType,
		nonce, destination string) ([]byte, error)
	Decrypt(ctx context.Context,
		cipher_text []byte) (*MessageInfo, error)
}

type IClientCryptoManager interface {
	ICryptoManager
	AddCertificate(config_obj *config_proto.Config, certificate_pem []byte) (string, error)
}
