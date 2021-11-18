package services

import (
	"sync"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
)

var (
	client_info_manager    ClientInfoManager
	client_info_manager_mu sync.Mutex
)

const (
	Unknown ClientOS = iota
	Windows
	Linux
	MacOS
)

type ClientOS int

func GetClientInfoManager() ClientInfoManager {
	client_info_manager_mu.Lock()
	defer client_info_manager_mu.Unlock()

	return client_info_manager
}

func RegisterClientInfoManager(m ClientInfoManager) {
	client_info_manager_mu.Lock()
	defer client_info_manager_mu.Unlock()

	client_info_manager = m
}

type ClientInfo struct {
	// The original info from disk
	actions_proto.ClientInfo
}

func (self ClientInfo) Copy() ClientInfo {
	copy := proto.Clone(&self.ClientInfo).(*actions_proto.ClientInfo)
	self.ClientInfo = *copy
	return self
}

func (self ClientInfo) OS() ClientOS {
	switch self.System {
	case "windows":
		return Windows
	case "linux":
		return Linux
	case "darwin":
		return MacOS
	}
	return Unknown
}

type ClientInfoManager interface {
	UpdatePing(client_id, ip_address string) error
	Get(client_id string) (*ClientInfo, error)

	// Remove client id from the cache - this is needed when the
	// record chages and we need to force a read from storage.
	Flush(client_id string)
}

func GetHostname(client_id string) string {
	client_info_manager := GetClientInfoManager()
	if client_info_manager == nil {
		return ""
	}
	info, err := client_info_manager.Get(client_id)
	if err != nil {
		return ""
	}

	return info.Hostname
}
