package networking

import (
	"time"

	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

// Addr is implemented compatibility to psutil
type Addr struct {
	IP   string
	Port uint32
}

type ConnectionStat struct {
	Fd        uint32
	Family    uint32
	Type      uint32
	Laddr     Addr
	Raddr     Addr
	Status    string
	Pid       int32
	timestamp time.Time
}

var _Netstat = vfilter.GenericListPlugin{
	PluginName: "netstat",
	Doc:        "Collect network information.",
	Function:   runNetstat,
	ArgType:    &NetstatArgs{},
	Metadata:   vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
}

type NetstatArgs struct{}
