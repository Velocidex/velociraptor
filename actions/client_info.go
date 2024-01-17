package actions

import (
	"context"
	"runtime"

	"github.com/Showmax/go-fqdn"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
)

// Return essential information about the client used for indexing
// etc. This augments the interrogation workflow via the
// Server.Internal.ClientInfo artifact. We send this message to the
// server periodically to avoid having to issue Generic.Client.Info
// hunts all the time.
func GetClientInfo(
	ctx context.Context,
	config_obj *config_proto.Config) *actions_proto.ClientInfo {
	result := &actions_proto.ClientInfo{}

	info, err := psutils.InfoWithContext(ctx)
	if err == nil {
		result = &actions_proto.ClientInfo{
			Hostname:     info.Hostname,
			System:       info.OS,
			Release:      info.Platform + info.PlatformVersion,
			Architecture: runtime.GOARCH,
			Fqdn:         fqdn.Get(),
		}
	}

	if config_obj.Version != nil {
		result.ClientName = config_obj.Version.Name
		result.ClientVersion = config_obj.Version.Version
		result.BuildUrl = config_obj.Version.CiBuildUrl
		result.BuildTime = config_obj.Version.BuildTime
		result.InstallTime = config_obj.Version.InstallTime
	}

	if config_obj.Client != nil {
		result.Labels = config_obj.Client.Labels
	}

	return result
}
