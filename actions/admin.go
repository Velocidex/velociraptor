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
)

type GetHostname struct{}

func (self *GetHostname) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(msg)

	info, err := host.Info()
	if err != nil {
		return responder.RaiseError(err.Error())
	}
	responder.AddResponse(&actions_proto.DataBlob{
		String_: proto.String(info.Hostname),
	})
	return responder.Return()
}

type GetPlatformInfo struct{}

func (self *GetPlatformInfo) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(msg)

	info, err := host.Info()
	if err != nil {
		return responder.RaiseError(err.Error())
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

	return responder.Return()
}
