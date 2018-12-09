package actions

import (
	"context"
	"runtime"
	"strings"

	"github.com/Showmax/go-fqdn"
	"github.com/shirou/gopsutil/host"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

type GetHostname struct{}

func (self *GetHostname) Run(
	config *api_proto.Config,
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
	config *api_proto.Config,
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
