/*
 Mediate access to the client queue through the client info
 cache. Using this we can reduce datastore IO by caching the client's
 tasks.
*/

package client_info

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
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

	clientCancellationCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "client_flow_cancellations",
		Help: "Total number of client cancellation messages sent.",
	})
)

func (self *ClientInfoManager) ProcessNotification(
	ctx context.Context, config_obj *config_proto.Config,
	row *ordereddict.Dict) error {
	client_id, pres := row.GetString("ClientId")
	if pres {
		err := self.storage.Modify(ctx, client_id,
			func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
				if client_info == nil {
					return nil, utils.NotFoundError
				}
				client_info.HasTasks = true
				return client_info, nil
			})

		// If a record does not exist we ignore the notification.
		if err == nil {
			notifier, err := services.GetNotifier(config_obj)
			if err != nil {
				return err
			}
			notifier.NotifyDirectListener(client_id)
		}
	}
	return nil
}

func (self *ClientInfoManager) UnQueueMessageForClient(
	ctx context.Context, client_id string,
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
	ctx context.Context,
	client_id string,
	req []*crypto_proto.VeloMessage,
	/* Also notify the client about the new task */
	notify bool) error {

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// When the completer is done send a message to all the minions
	// that the tasks are ready to be read. This will cause all nodes
	// to update the client record's has_tasks field. On the master
	// node this information will be flushed on the next snapshot
	// write.
	completer := utils.NewCompleter(func() {
		err := self.storage.Modify(ctx, client_id,
			func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
				if client_info == nil {
					return nil, utils.NotFoundError
				}
				client_info.HasTasks = true
				return client_info, nil
			})

		if err == nil {
			journal.PushRowsToArtifactAsync(ctx, self.config_obj,
				ordereddict.NewDict().
					Set("ClientId", client_id).
					Set("Notify", notify),
				"Server.Internal.ClientTasks")
		}
	})
	defer completer.GetCompletionFunc()()

	client_path_manager := paths.NewClientPathManager(client_id)

	for _, r := range req {
		task := proto.Clone(r).(*crypto_proto.VeloMessage)

		// Task ID is related to time.
		task.TaskId = currentTaskId()

		err = db.SetSubjectWithCompletion(self.config_obj,
			client_path_manager.Task(task.TaskId),
			task, completer.GetCompletionFunc())
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *ClientInfoManager) QueueMessageForClient(
	ctx context.Context,
	client_id string,
	req *crypto_proto.VeloMessage, notify bool,
	completion func()) error {

	if req.Cancel != nil {
		clientCancellationCounter.Inc()
	}

	// Task ID is related to time.
	req.TaskId = currentTaskId()

	journal, err := services.GetJournal(self.config_obj)
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

			// This message will be received by all nodes and cause
			// the client's recrod to update the has_tasks field.
			journal.PushRowsToArtifactAsync(ctx, self.config_obj,
				ordereddict.NewDict().
					Set("ClientId", client_id).
					Set("Notify", notify),
				"Server.Internal.ClientTasks")
		})
}

// Get the client tasks but do not dequeue them (Generally only called
// by tests).
func (self *ClientInfoManager) PeekClientTasks(ctx context.Context,
	client_id string) ([]*crypto_proto.VeloMessage, error) {

	record, err := self.storage.GetRecord(client_id)
	if err != nil {
		// Not an error if the client does not exist.
		return nil, nil
	}

	// This is by far the most common case - we know the client has no
	// tasks outstanding. We can return immediately without any IO
	if !record.HasTasks {
		return nil, nil
	}

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

		message := &crypto_proto.VeloMessage{}
		err = db.GetSubject(self.config_obj, task_urn, message)
		if err != nil {
			continue
		}
		result = append(result, message)
	}

	return result, nil
}

var (
	noTasksError = errors.New("No Tasks")
)

// Gets all the tasks from the client and remove from the datastore.
func (self *ClientInfoManager) GetClientTasks(
	ctx context.Context, client_id string) (
	[]*crypto_proto.VeloMessage, error) {

	err := self.storage.Modify(ctx, client_id,
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			if client_info == nil {
				return nil, utils.NotFoundError
			}
			if !client_info.HasTasks {
				return nil, noTasksError
			}
			// Reset the HasTasks flag
			client_info.HasTasks = false
			return client_info, nil
		})

	if err == utils.NotFoundError {
		// Not an error if the client does not exist.
		return nil, nil
	}

	// This is by far the most common case - we know the client has no
	// tasks outstanding. We can return immediately without any IO
	if err == noTasksError {
		return nil, nil
	}

	var tasks []api.DSPathSpec

	// Fetch all the tasks.
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

	result := []*crypto_proto.VeloMessage{}
	for _, task_urn := range tasks {
		task_urn = task_urn.SetTag("ClientTask")

		// Here we read the task from the task_urn and remove
		// it from the queue.
		message := &crypto_proto.VeloMessage{}
		err = db.GetSubject(self.config_obj, task_urn, message)
		if err != nil {
			// The file seems invalid, just delete it.
			_ = db.DeleteSubject(self.config_obj, task_urn)
			continue
		}

		err = db.DeleteSubject(self.config_obj, task_urn)
		if err != nil {
			return nil, err
		}

		// Handle backwards compatibility with older clients by expanding
		// FlowRequest into separate VQLClientActions. Newer clients will
		// ignore bare VQLClientActions and older clients will ignore
		// FlowRequest.
		if message.FlowRequest != nil &&
			len(message.FlowRequest.VQLClientActions) > 0 {

			// Tack the first VQLClientAction on top of the
			// FlowRequest for backwards compatibility. Newer clients
			// procees FlowRequest first and ignore VQLClientAction
			// while older clients will process the VQLClientAction
			// and ignore the FlowRequest message. In both cases the
			// message will be valid.
			message.VQLClientAction = proto.Clone(
				message.FlowRequest.VQLClientActions[0]).(*actions_proto.VQLCollectorArgs)

			// Send the rest of the VQLClientAction as distinct messages.
			for idx, request := range message.FlowRequest.VQLClientActions {
				if idx > 0 {
					result = append(result, &crypto_proto.VeloMessage{
						SessionId:       message.SessionId,
						RequestId:       message.RequestId,
						VQLClientAction: request,
					})
				}
			}
		}

		result = append(result, message)
	}

	return result, nil
}

func currentTaskId() uint64 {
	id := atomic.AddUint64(&g_id, 1)
	return uint64(Clock.Now().UTC().UnixNano()&0x7fffffffffff0000) | (id & 0xFFFF)
}
