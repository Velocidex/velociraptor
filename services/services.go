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
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/users"
)

// A manager responsible for starting and shutting down all the
// services in an orderly fashion.
type ServicesManager struct {
	hunt_manager    *HuntManager
	hunt_dispatcher *HuntDispatcher
	user_manager    *users.UserNotificationManager
	stats_collector *StatsCollector
}

func (self *ServicesManager) Close() {
	self.hunt_manager.Close()
	self.hunt_dispatcher.Close()
	self.user_manager.Close()
	self.stats_collector.Close()
}

// Start all the server services.
func StartServices(config_obj *api_proto.Config) (*ServicesManager, error) {
	result := &ServicesManager{}

	hunt_manager, err := startHuntManager(config_obj)
	if err != nil {
		return nil, err
	}
	result.hunt_manager = hunt_manager

	hunt_dispatcher, err := startHuntDispatcher(config_obj)
	if err != nil {
		return nil, err
	}
	result.hunt_dispatcher = hunt_dispatcher

	user_manager, err := users.StartUserNotificationManager(config_obj)
	if err != nil {
		return nil, err
	}
	result.user_manager = user_manager

	stats_collector, err := startStatsCollector(config_obj)
	if err != nil {
		return nil, err
	}
	result.stats_collector = stats_collector

	return result, nil
}
