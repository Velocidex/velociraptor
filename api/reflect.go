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
package api

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/lsp"
)

func (self *ApiServer) GetKeywordCompletions(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.KeywordCompletions, error) {

	defer Instrument("GetKeywordCompletions")()

	users := services.GetUserManager()
	_, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &api_proto.KeywordCompletions{
		Items: []*api_proto.Completion{
			{Name: "EXPLAIN", Type: "Keyword"},
			{Name: "SELECT", Type: "Keyword"},
			{Name: "FROM", Type: "Keyword"},
			{Name: "LET", Type: "Keyword"},
			{Name: "WHERE", Type: "Keyword"},
			{Name: "LIMIT", Type: "Keyword"},
			{Name: "GROUP BY", Type: "Keyword"},
			{Name: "ORDER BY", Type: "Keyword"},
			{Name: "DESC", Type: "Keyword"},
		},
	}

	result.Items = append(result.Items, lsp.LoadApiDescriptions()...)
	artifact_desc, err := lsp.LoadArtifactDescriptions(
		ctx, org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result.Items = append(result.Items, artifact_desc...)

	return result, nil
}
