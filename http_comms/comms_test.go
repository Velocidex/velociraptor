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
package http_comms

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
)

func TestHTTPComms(t *testing.T) {
	config_obj, err := config.LoadConfig("test_data/client.config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := crypto.NewClientCryptoManager(
		config_obj, []byte(config_obj.Writeback.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}

	exe, err := executor.NewClientExecutor(config_obj)
	if err != nil {
		t.Fatal(err)
	}

	comm, err := NewHTTPCommunicator(
		config_obj,
		manager,
		exe,
		[]string{
			"http://localhost:8080/",
		})
	if err != nil {
		t.Fatal(err)
	}

	_ = comm
	//	comm.Run()
}
