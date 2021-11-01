package services

import (
	"sync"

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
	Hostname string
	OS       ClientOS

	Info *actions_proto.ClientInfo
}

func (self ClientInfo) OSString() string {
	switch self.OS {
	case Windows:
		return "windows"
	case Linux:
		return "Linux"
	case MacOS:
		return "MacOS"
	}
	return "Unknown"
}

type ClientInfoManager interface {
	Get(client_id string) (*ClientInfo, error)
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
