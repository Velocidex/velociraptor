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
package services

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/notifications"
	"www.velocidex.com/golang/velociraptor/users"
)

// A manager responsible for starting and shutting down all the
// services in an orderly fashion.
type ServicesManager struct{}

// Start all the server services.
func StartServices(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	notifier *notifications.NotificationPool) error {

	// Start critical services first.
	err := StartJournalService(config_obj)
	if err != nil {
		return err
	}

	err = startNotificationService(config_obj, notifier)
	if err != nil {
		return err
	}

	if config_obj.ServerServices.HuntManager {
		_, err := startHuntManager(ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.HuntDispatcher {
		_, err := StartHuntDispatcher(ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.UserManager {
		err := users.StartUserNotificationManager(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.StatsCollector {
		err := startStatsCollector(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.ServerMonitoring {
		err := startServerMonitoringService(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.ServerArtifacts {
		err := startServerArtifactService(
			ctx, wg, config_obj, notifier)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.ClientMonitoring {
		err := StartClientMonitoringService(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.DynDns {
		err := startDynDNSService(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.Interrogation {
		startInterrogationService(ctx, wg, config_obj)
	}

	if config_obj.ServerServices.SanityChecker {
		err := startSanityCheckService(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.ServerServices.VfsService {
		err := startVFSService(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	return nil
}
