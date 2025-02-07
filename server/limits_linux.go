//go:build linux
// +build linux

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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

package server

import (
	"syscall"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

// On linux we can use a system call to increase our file handle
// limits. Velociraptor needs more file handles because our comms
// protocol holds sockets open for a long time. Therefore we need to
// hold at least 2 sockets for each provisioned client. By default the
// limit is very low (4096 or 1024) and so we need to increase it.

// When running as root we may increase the limit ourselves. Otherwise
// we may only increase it up to the hard limit.
func IncreaseLimits(config_obj *config_proto.Config) {
	var rLimit syscall.Rlimit

	logger := logging.GetLogger(config_obj,
		&logging.FrontendComponent)

	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		logger.Info("Error Getting Rlimit %v", err)
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
