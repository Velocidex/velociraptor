package actions

import (
	"runtime"

	"github.com/Showmax/go-fqdn"
	"github.com/shirou/gopsutil/v3/host"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Return essential information about the client used for indexing
// etc. This augments the interrogation workflow via the
// Server.Internal.ClientInfo artifact. We send this message tothe
// server periodically to avoid having to issue Generic.Client.Info
// hunts all the time.
func GetClientInfo(
	config_obj *config_proto.Config) *actions_proto.ClientInfo {
	result := &actions_proto.ClientInfo{}

	info, err := host.Info()
	if err == nil && config_obj.Version != nil {
		result = &actions_proto.ClientInfo{
			Hostname:      info.Hostname,
			System:        info.OS,
			Release:       info.Platform,
			Architecture:  runtime.GOARCH,
			Fqdn:          fqdn.Get(),
			ClientName:    config_obj.Version.Name,
			ClientVersion: config_obj.Version.Version,
			BuildUrl:      config_obj.Version.CiBuildUrl,
			BuildTime:     config_obj.Version.BuildTime,
		}
	}

	return result
}
