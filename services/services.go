package services

import api_proto "www.velocidex.com/golang/velociraptor/api/proto"

// A manager responsible for starting and shutting down all the
// services in an orderly fashion.
type ServicesManager struct {
	hunt_manager    *HuntManager
	hunt_dispatcher *HuntDispatcher
}

func (self *ServicesManager) Close() {
	self.hunt_manager.Close()
	self.hunt_dispatcher.Close()
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

	return result, nil
}
