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
package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

func enroll(
	ctx context.Context,
	config_obj *config_proto.Config,
	server *Server,
	csr *crypto_proto.Certificate) error {

	if csr.GetType() != crypto_proto.Certificate_CSR || csr.Pem == nil {
		return nil
	}

	client_id, err := server.manager.AddCertificateRequest(config_obj, csr.Pem)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("While enrolling %v: %v", client_id, err)
		return err
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("ClientId", client_id)},
		"Server.Internal.Enrollment", client_id, "")
}
