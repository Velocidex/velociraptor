//go:build windows
// +build windows

package logging

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

// Syslog is not supported on Windows.
func maybeAddRemoteSyslog(
	config_obj *config_proto.Config, manager *LogManager) error {
	return nil
}
