// Manage the client task queues

package clients

import (
	"sync/atomic"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	Clock utils.Clock = &utils.RealClock{}
	g_id  uint64
)

func GetClientTasks(
	config_obj *config_proto.Config,
	client_id string,
	do_not_lease bool) ([]*crypto_proto.VeloMessage, error) {
	result := []*crypto_proto.VeloMessage{}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	tasks, err := db.ListChildren(
		config_obj, client_path_manager.TasksDirectory())
	if err != nil {
		return nil, err
	}

	for _, task_urn := range tasks {
		task_urn = task_urn.SetTag("ClientTask")

		// Here we read the task from the task_urn and remove
		// it from the queue.
		message := &crypto_proto.VeloMessage{}
		err = db.GetSubject(config_obj, task_urn, message)
		if err != nil {
			continue
		}

		if !do_not_lease {
			err = db.DeleteSubject(config_obj, task_urn)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, message)
	}
	return result, nil
}

func UnQueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	message *crypto_proto.VeloMessage) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	return db.DeleteSubject(config_obj,
		client_path_manager.Task(message.TaskId))
}

func currentTaskId() uint64 {
	id := atomic.AddUint64(&g_id, 1)
	return uint64(Clock.Now().UTC().UnixNano()&0x7fffffffffff0000) | (id & 0xFFFF)
}

func QueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	req *crypto_proto.VeloMessage) error {

	// Task ID is related to time.
	req.TaskId = currentTaskId()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	return db.SetSubject(config_obj,
		client_path_manager.Task(req.TaskId), req)
}
