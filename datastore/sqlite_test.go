// +build sqlite

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

package datastore

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/config"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

func TestPaths(t *testing.T) {
	assert.Equal(
		t,
		"test_data/C%2Eb4f82077e4af5ba7.sqlite",
		getDBPathForClient("test_data", "C.b4f82077e4af5ba7"),
	)
}

func xxxxTestMain(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore_location = proto.String("/tmp/velociraptor")

	db, pres := GetImpl("SqliteDataStore")
	if !pres {
		t.Fatalf("No such implementation")
	}

	messages, err := db.GetClientTasks(config_obj, "C.b4f82077e4af5ba7", false)
	if err != nil {
		t.Fatalf(err.Error())
	}

	utils.Debug(messages)
}
