/*
 Mediate access to the client queue through the client info
 cache. Using this we can reduce datastore IO by caching the client's
 tasks.
*/

package client_info

import (
	"context"
	"sync/atomic"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	tasksClearCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "client_info_client_tasks_notifications",
			Help: "Number of notifications received that clients have new tasks",
		})

	Clock utils.Clock = &utils.RealClock{}
	g_id  uint64
)

type TASKS_AVAILABLE_STATUS int

const (
	TASKS_AVAILABLE_STATUS_UNKNOWN TASKS_AVAILABLE_STATUS = iota
	TASKS_AVAILABLE_STATUS_YES
	TASKS_AVAILABLE_STATUS_NO
)

func (self *ClientInfoManager) ProcessNotification(
	ctx context.Context, config_obj *config_proto.Config,
	row *ordereddict.Dict) error {
	client_id, pres := row.GetString("ClientId")
	if pres {
		cached_info, err := self.GetCacheInfo(client_id)
		if err != nil {
			return err
		}

		// Next access will check for real.
		tasksClearCount.Inc()
		cached_info.SetHasTasks(TASKS_AVAILABLE_STATUS_YES)

		notifier := services.GetNotifier()
		if notifier != nil {
			notifier.NotifyDirectListener(client_id)
		}
		utils.DebugToFile("/tmp/frontend_notify.txt", "ClientInfoManager notification for %v", client_id)
	}
	return nil
}

func (self *ClientInfoManager) UnQueueMessageForClient(
	client_id string,
	message *crypto_proto.VeloMessage) error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	return db.DeleteSubject(self.config_obj,
		client_path_manager.Task(message.TaskId))
}

func (self *ClientInfoManager) QueueMessagesForClient(
	client_id string,
	req []*crypto_proto.VeloMessage,
	/* Also notify the client about the new task */
	notify bool) error {

	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// When the completer is done send a message to all the minions
	// that the tasks are ready to be read.
	completer := utils.NewCompleter(func() {
		journal.PushRowsToArtifactAsync(self.config_obj,
			ordereddict.NewDict().
				Set("ClientId", client_id).
				Set("Notify", notify),
			"Server.Internal.ClientTasks")
	})
	defer completer.GetCompletionFunc()()

	client_path_manager := paths.NewClientPathManager(client_id)

	for _, r := range req {
		// Task ID is related to time.
		r.TaskId = currentTaskId()

		err = db.SetSubjectWithCompletion(self.config_obj,
			client_path_manager.Task(r.TaskId),
			r, completer.GetCompletionFunc())
	}
	return nil
}

func (self *ClientInfoManager) QueueMessageForClient(
	client_id string,
	req *crypto_proto.VeloMessage, notify bool,
	completion func()) error {

	// Task ID is related to time.
	req.TaskId = currentTaskId()

	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	return db.SetSubjectWithCompletion(self.config_obj,
		client_path_manager.Task(req.TaskId),
		req, func() {
			if completion != nil {
				completion()
			}

			journal.PushRowsToArtifactAsync(self.config_obj,
				ordereddict.NewDict().
					Set("ClientId", client_id).
					Set("Notify", notify),
				"Server.Internal.ClientTasks")
		})
}

// Get the client tasks but do not dequeue them (Generally only called
// by tests).
func (self *ClientInfoManager) PeekClientTasks(client_id string) (
	[]*crypto_proto.VeloMessage, error) {

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}
	client_path_manager := paths.NewClientPathManager(client_id)
	tasks, err := db.ListChildren(
		self.config_obj, client_path_manager.TasksDirectory())
	if err != nil {
		return nil, err
	}

	result := []*crypto_proto.VeloMessage{}
	for _, task_urn := range tasks {
		task_urn = task_urn.SetTag("ClientTask")

		// Here we read the task from the task_urn and remove
		// it from the queue.
		message := &crypto_proto.VeloMessage{}
		err = db.GetSubject(self.config_obj, task_urn, message)
		if err != nil {
			continue
		}
		result = append(result, message)
	}

	return result, nil
}

func (self *ClientInfoManager) GetClientTasks(client_id string) (
	[]*crypto_proto.VeloMessage, error) {
	cached_info, err := self.GetCacheInfo(client_id)
	if err != nil {
		return nil, nil
	}

	// This is by far the most common case - we know the client has no
	// tasks outstanding. We can return immediately without any IO
	if cached_info.GetHasTasks() == TASKS_AVAILABLE_STATUS_NO {
		return nil, nil
	}

	var tasks []api.DSPathSpec

	// We really do not know - lets check
	if cached_info.GetHasTasks() == TASKS_AVAILABLE_STATUS_UNKNOWN {
		db, err := datastore.GetDB(self.config_obj)
		if err != nil {
			return nil, err
		}

		client_path_manager := paths.NewClientPathManager(client_id)
		tasks, err = db.ListChildren(
			self.config_obj, client_path_manager.TasksDirectory())
		if err != nil {
			return nil, err
		}

		if len(tasks) > 0 {
			cached_info.SetHasTasks(TASKS_AVAILABLE_STATUS_YES)

		} else {
			// No tasks available.
			cached_info.SetHasTasks(TASKS_AVAILABLE_STATUS_NO)
			return nil, nil
		}
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	// We know that there are tasks available, let's get them.
	if len(tasks) == 0 {
		client_path_manager := paths.NewClientPathManager(client_id)
		tasks, err = db.ListChildren(
			self.config_obj, client_path_manager.TasksDirectory())
		if err != nil {
			return nil, err
		}
	}

	result := []*crypto_proto.VeloMessage{}
	for _, task_urn := range tasks {
		task_urn = task_urn.SetTag("ClientTask")

		// Here we read the task from the task_urn and remove
		// it from the queue.
		message := &crypto_proto.VeloMessage{}
		err = db.GetSubject(self.config_obj, task_urn, message)
		if err != nil {
			continue
		}

		err = db.DeleteSubject(self.config_obj, task_urn)
		if err != nil {
			return nil, err
		}
		result = append(result, message)
	}

	// No more tasks available.
	cached_info.SetHasTasks(TASKS_AVAILABLE_STATUS_NO)

	return result, nil
}

func currentTaskId() uint64 {
	id := atomic.AddUint64(&g_id, 1)
	return uint64(Clock.Now().UTC().UnixNano()&0x7fffffffffff0000) | (id & 0xFFFF)
}
