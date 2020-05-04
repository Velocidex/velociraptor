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
)

// A manager responsible for starting and shutting down all the
// services in an orderly fashion.
type ServicesManager struct{}

// Some services run on all frontends. These must all run without
// exception so they can not be selectively started.
func StartFrontendServices(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	// Allow for low latency scheduling by notifying clients of
	// new events for them.
	err := StartNotificationService(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	// Hunt dispatcher manages client's hunt membership.
	_, err = StartHuntDispatcher(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	// Maintans the client's event monitoring table. All frontends
	// need to follow this so they can propagate changes to
	// clients.
	err = StartClientMonitoringService(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	// Updates DynDNS records if needed. Frontends need to maintain their IP addresses.
	err = startDynDNSService(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	// Check everything is ok before we can start.
	err = startSanityCheckService(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	return nil
}

// Start all the server services.
func StartServices(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if config_obj.Frontend.ServerServices.HuntManager {
		_, err := startHuntManager(ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	/* Not really implemented - do we really need it any more?

	if config_obj.ServerServices.UserManager {
		err := users.StartUserNotificationManager(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}
	*/

	// The stats collector runs periodically and reports 1, 7 and
	// 30 day active clients.
	if config_obj.Frontend.ServerServices.StatsCollector {
		err := startStatsCollector(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	// Runs server event queries. Should only run on one frontend.
	if config_obj.Frontend.ServerServices.ServerMonitoring {
		err := startServerMonitoringService(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	// Run any server arttifacts the user asks for.
	if config_obj.Frontend.ServerServices.ServerArtifacts {
		err := startServerArtifactService(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	// Interrogation service populates indexes etc for new
	// clients.
	if config_obj.Frontend.ServerServices.Interrogation {
		startInterrogationService(ctx, wg, config_obj)
	}

	// VFS service maintains the VFS GUI structures by parsing the
	// output of VFS artifacts collected.
	if config_obj.Frontend.ServerServices.VfsService {
		err := startVFSService(
			ctx, wg, config_obj)
		if err != nil {
			return err
		}
	}

	return nil
}
