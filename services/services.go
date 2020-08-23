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

var (
	service_mu     sync.Mutex
	ServiceManager *Service
)

func NewServiceManager(ctx context.Context,
	config_obj *config_proto.Config) *Service {
	service_mu.Lock()
	defer service_mu.Unlock()

	self := &Service{Config: config_obj, Wg: &sync.WaitGroup{}}
	self.Ctx, self.cancel = context.WithCancel(ctx)

	ServiceManager = self
	return self
}

type Service struct {
	Ctx    context.Context
	cancel func()
	Wg     *sync.WaitGroup
	Config *config_proto.Config
}

func (self *Service) Close() {
	self.cancel()

	self.Wg.Wait()
}

type StarterFunc func(ctx context.Context, wg *sync.WaitGroup, config_obj *config_proto.Config) error

func (self *Service) Start(starter StarterFunc) error {
	return starter(self.Ctx, self.Wg, self.Config)
}
