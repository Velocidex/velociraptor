package actions

import (
	"github.com/Showmax/go-fqdn"
	"github.com/golang/protobuf/proto"
	"github.com/shirou/gopsutil/host"
	"runtime"
	"strings"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

type GetHostname struct{}

func (self *GetHostname) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {

	responder := responder.NewResponder(msg, output)

	info, err := host.Info()
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}
	responder.AddResponse(&actions_proto.DataBlob{
		String_: proto.String(info.Hostname),
	})

	responder.Return()
}

type GetPlatformInfo struct{}

func (self *GetPlatformInfo) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)

	info, err := host.Info()
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}
	responder.AddResponse(&actions_proto.Uname{
		System:       proto.String(strings.Title(info.OS)),
		Fqdn:         proto.String(fqdn.Get()),
		Architecture: proto.String(runtime.GOARCH),
		Release:      proto.String(info.Platform),
		Version:      proto.String(info.PlatformVersion),
		Kernel:       proto.String(info.KernelVersion),
		Pep425Tag: proto.String("Golang_" + info.OS + "_" +
			info.Platform + "_" + info.PlatformVersion),
	})

	responder.Return()
}
