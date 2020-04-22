package paths

import (
	"fmt"
	"path"
)

func GetClientMetadataPath(client_id string) string {
	return path.Join("/clients", client_id)
}

func GetClientPingPath(client_id string) string {
	return path.Join("/clients", client_id, "ping")
}

func GetClientKeyPath(client_id string) string {
	return path.Join("/clients", client_id, "key")
}

func GetClientTasksPath(client_id string) string {
	return path.Join("/clients", client_id, "tasks")
}

func GetClientTaskPath(client_id string, task_id uint64) string {
	return path.Join("/clients", client_id, "tasks",
		fmt.Sprintf("%d", task_id))
}
