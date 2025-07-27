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
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
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
		err := self.storage.Modify(ctx, self.config_obj, client_id,
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
		err := self.storage.Modify(ctx, self.config_obj, client_id,
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

		if notify {
			notifier, err := services.GetNotifier(self.config_obj)
			if err != nil {
				return
			}

			notifier.NotifyDirectListener(client_id)
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

	completer := utils.NewCompleter(func() {
		if completion != nil &&
			!utils.CompareFuncs(completion, utils.SyncCompleter) {
			completion()
		}

		err := self.storage.Modify(ctx, self.config_obj, client_id,
			func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
				if client_info == nil {
					return nil, utils.NotFoundError
				}
				client_info.HasTasks = true

				return client_info, nil
			})

		if err != nil {
			return
		}

		journal.PushRowsToArtifactAsync(ctx, self.config_obj,
			ordereddict.NewDict().
				Set("ClientId", client_id).
				Set("Notify", notify),
			"Server.Internal.ClientTasks")

		if notify {
			notifier, err := services.GetNotifier(self.config_obj)
			if err != nil {
				return
			}

			notifier.NotifyDirectListener(client_id)
		}
	})
	defer completer.GetCompletionFunc()()

	client_path_manager := paths.NewClientPathManager(client_id)
	return db.SetSubjectWithCompletion(self.config_obj,
		client_path_manager.Task(req.TaskId),
		req, completer.GetCompletionFunc())
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

// Fetch the next number of flow_request tasks off the queue and
// dequeue them. NOTE: This function can return more than number
// messages but only number FlowRequest objects.
func (self *ClientInfoManager) getClientTasks(
	ctx context.Context, client_id string, number int) (
	[]*crypto_proto.VeloMessage, error) {

	var result []*crypto_proto.VeloMessage
	var tasks []api.DSPathSpec
	total_flow_requests := 0
	more_requests_available := false

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

		// Handle backwards compatibility with older clients by expanding
		// FlowRequest into separate VQLClientActions. Newer clients will
		// ignore bare VQLClientActions and older clients will ignore
		// FlowRequest.
		if message.FlowRequest != nil {
			total_flow_requests++

			// Only include the first number requests, unless they are
			// urgent requests which are always delivered regardless.
			if total_flow_requests <= number || message.Urgent {
				result = append(result, message)

				// Add extra backwards compatibility messages for
				// older clients.
				if len(message.FlowRequest.VQLClientActions) > 0 {

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

				// Delete the scheduled tasks
				err = db.DeleteSubject(self.config_obj, task_urn)
				if err != nil {
					return nil, err
				}
			} else {
				// Skip additional FlowRequest packets
				more_requests_available = true
				continue
			}

		} else {
			// Non FlowRequest packets are always included
			// (e.g. Cancel)
			result = append(result, message)

			// Delete the scheduled tasks
			err = db.DeleteSubject(self.config_obj, task_urn)
			if err != nil {
				return nil, err
			}
		}

	}

	if more_requests_available {
		client_info_manager, err := services.GetClientInfoManager(self.config_obj)
		if err != nil {
			return nil, err
		}

		err = client_info_manager.Modify(ctx, client_id,
			func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
				if client_info == nil {
					client_info = &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
					client_info.ClientId = client_id
				}

				client_info.HasTasks = true
				return client_info, nil
			})
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// Gets the tasks from the client queue and remove from the datastore.
func (self *ClientInfoManager) GetClientTasks(
	ctx context.Context, client_id string) (
	[]*crypto_proto.VeloMessage, error) {

	// This list holds the flows that are inflight and we have not
	// heard from them for some time. We can actively request the
	// client to report on them again to see how they are going.
	var inflight_notifications []string

	// Number of currently in flight flows to the client. This is the
	// total number, including those that were recently scheduled - it
	// is not the same as len(inflight_notifications)
	inflight_requests := 0

	inflight_checks_enabled := true
	inflight_check_time := int64(60)
	if self.config_obj.Defaults != nil {
		if self.config_obj.Defaults.DisableActiveInflightChecks {
			inflight_checks_enabled = false
		}

		if self.config_obj.Defaults.InflightCheckTime > 0 {
			inflight_check_time = self.config_obj.Defaults.InflightCheckTime
		}

	}

	now := utils.GetTime().Now().Unix()

	err := self.storage.Modify(ctx, self.config_obj, client_id,
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			if client_info == nil {
				return nil, utils.NotFoundError
			}

			// Gather up any stats notifications we might have
			inflight_requests = len(client_info.InFlightFlows)

			// No tasks to send and we dont have anything in flight -
			// just exit quickly.
			if !client_info.HasTasks && inflight_requests == 0 {
				return nil, noTasksError
			}

			// Check up on in flight flows every 60 sec at least
			// (could be more depending on poll).
			if inflight_checks_enabled {
				for k, v := range client_info.InFlightFlows {
					if now-v > inflight_check_time {
						inflight_notifications = append(
							inflight_notifications, k)
					}
				}

				// Update the time to ensure we dont send these too often.
				for _, k := range inflight_notifications {
					client_info.InFlightFlows[k] = utils.GetTime().Now().Unix()
				}
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

	var result []*crypto_proto.VeloMessage

	// We only allow a maximum of 2 tasks above client concurrency to
	// be in flight at any time. We know there are some requests
	// already in flight, so we only fetch as many as we need.
	max_inflight_requests := 4
	if self.config_obj.Client != nil && self.config_obj.Client.Concurrency > 0 {
		max_inflight_requests = 2 + int(self.config_obj.Client.Concurrency)
	}

	result, err = self.getClientTasks(ctx, client_id,
		max_inflight_requests-inflight_requests)
	if err != nil {
		return nil, err
	}

	// Add a notification request to the client asking about the
	// status of currently in flight requests.
	if len(inflight_notifications) > 0 {
		launcher, err := services.GetLauncher(self.config_obj)
		if err != nil {
			return nil, err
		}

		// Check the launcher if the flows are really in flight or
		// were they already resolved.
		verified := make([]string, 0, len(inflight_notifications))
		resolved := make([]string, 0, len(inflight_notifications))
		for _, n := range inflight_notifications {
			// Only request status for flows that have not actually
			// been completed.
			flow_obj, err := launcher.GetFlowDetails(ctx, self.config_obj,
				services.GetFlowOptions{}, client_id, n)
			if err != nil {
				// The flow can not be loaded - we can not check up on
				// it any more - remove it from the in flight set.
				resolved = append(resolved, n)
				continue
			}

			// If the flow is resolved we ignore it.
			switch flow_obj.Context.State {
			case flows_proto.ArtifactCollectorContext_FINISHED,
				flows_proto.ArtifactCollectorContext_ERROR:
				resolved = append(resolved, n)
			default:
				// All other flow states are still unclear what is
				// happening with it?
				verified = append(verified, n)
			}
		}

		if len(resolved) > 0 {
			err := self.storage.Modify(ctx, self.config_obj, client_id,
				func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
					if client_info == nil ||
						client_info.InFlightFlows == nil {
						return nil, nil
					}

					for _, k := range resolved {
						delete(client_info.InFlightFlows, k)
					}
					return client_info, nil
				})
			if err != nil {
				return nil, err
			}
		}

		// Ask the client about those flows
		if len(verified) > 0 {
			result = append(result, &crypto_proto.VeloMessage{
				SessionId: constants.STATUS_CHECK_WELL_KNOWN_FLOW,
				FlowStatsRequest: &crypto_proto.FlowStatsRequest{
					FlowId: verified,
				},
			})
		}
	}

	// What new flows were added?
	var inflight_flows []string
	for _, message := range result {
		// Filter out the FlowRequest checks
		if message.FlowRequest != nil && message.SessionId != "" {
			inflight_flows = append(inflight_flows, message.SessionId)
		}
	}

	if inflight_checks_enabled && len(inflight_flows) > 0 {

		// Add the inflight tags to the client record immediately.
		err := self.storage.Modify(ctx, self.config_obj, client_id,
			func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
				if client_info == nil {
					return nil, nil
				}

				if client_info.InFlightFlows == nil {
					client_info.InFlightFlows = make(map[string]int64)
				}

				for _, flow_id := range inflight_flows {
					client_info.InFlightFlows[flow_id] = now
				}

				return client_info, nil
			})
		if err != nil {
			return nil, err
		}

		// Message all the other nodes that these new flows are in
		// flight. The event listener will add them to the client info
		// service.
		journal, err := services.GetJournal(self.config_obj)
		if err != nil {
			return nil, err
		}

		journal.PushRowsToArtifactAsync(ctx, self.config_obj,
			ordereddict.NewDict().
				Set("ClientId", client_id).
				Set("InFlight", inflight_flows),
			"Server.Internal.ClientScheduled")
	}

	return result, nil
}

func currentTaskId() uint64 {
	id := atomic.AddUint64(&g_id, 1)
	return uint64(Clock.Now().UTC().UnixNano()&0x7fffffffffff0000) | (id & 0xFFFF)
}
