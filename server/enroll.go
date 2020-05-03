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
package server

import (
	"context"

	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

func enroll(
	ctx context.Context,
	server *Server,
	csr *crypto_proto.Certificate) error {
	if csr.GetType() == crypto_proto.Certificate_CSR && csr.Pem != nil {
		client_id, err := server.manager.AddCertificateRequest(csr.Pem)
		if err != nil {
			return err
		}

		// Schedule a collection automatically.
		_, err = flows.ScheduleArtifactCollection(server.config,
			server.config.Client.PinnedServerName,
			&flows_proto.ArtifactCollectorArgs{
				ClientId:  client_id,
				Artifacts: []string{constants.CLIENT_INFO_ARTIFACT},
			})
		if err != nil {
			return err
		}
	}

	return nil
}
