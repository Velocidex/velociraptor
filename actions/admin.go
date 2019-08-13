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
package actions

import (
	"context"
	"runtime"
	"strings"

	fqdn "github.com/Showmax/go-fqdn"
	"github.com/shirou/gopsutil/host"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

type GetHostname struct{}

func (self *GetHostname) Run(
	config *config_proto.Config,
	ctx context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {

	responder := responder.NewResponder(msg, output)

	info, err := host.Info()
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}
	responder.AddResponse(&actions_proto.DataBlob{
		String_: info.Hostname,
	})

	responder.Return()
}

type GetPlatformInfo struct{}

func (self *GetPlatformInfo) Run(
	config *config_proto.Config,
	ctx context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)

	info, err := host.Info()
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}
	responder.AddResponse(&actions_proto.Uname{
		System:       strings.Title(info.OS),
		Fqdn:         fqdn.Get(),
		Architecture: runtime.GOARCH,
		Release:      info.Platform,
		Version:      info.PlatformVersion,
		Kernel:       info.KernelVersion,
		Pep425Tag: "Golang_" + info.OS + "_" +
			info.Platform + "_" + info.PlatformVersion,
	})

	responder.Return()
}
