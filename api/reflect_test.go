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
package api

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	assert "github.com/stretchr/testify/assert"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
)

func TestDescriptor(t *testing.T) {
	result := &artifacts_proto.Types{}
	seen := make(map[string]bool)
	add_type("proto.ApiClient", result, seen)

	marshaler := &jsonpb.Marshaler{Indent: " "}
	str, err := marshaler.MarshalToString(result)
	assert.NoError(t, err)

	fmt.Println(str)
}
