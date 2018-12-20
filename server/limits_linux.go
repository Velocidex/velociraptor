// +build linux

package server

import (
	"syscall"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

// On linux we can use a system call to increase our file handle
// limits. Velociraptor needs more file handles because our comms
// protocol holds sockets open for a long time. Therefore we need to
// hold at least 2 sockets for each provisioned client. By default the
// limit is very low (4096 or 1024) and so we need to increase it.

// When running as root we may increase the limit ourselves. Otherwise
// we may only increase it up to the hard limit.
func IncreaseLimits(config_obj *api_proto.Config) {
	var rLimit syscall.Rlimit

	logger := logging.GetLogger(config_obj,
		&logging.FrontendComponent)

	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		logger.Info("Error Getting Rlimit ", err)
		return
	}
	rLimit.Max = 999999
	rLimit.Cur = 999999

	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		logger.Info("Error increasing limit %v. "+
			"This might work better as root.", err)
		return
	}

	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		return
	}

	logger.Info("Increased open file limit to %v", rLimit.Cur)
}
