package artifacts

import (
	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths/artifact_modes"
	"www.velocidex.com/golang/velociraptor/services"
)

// Only accept events from the server. This is appropriate for
// internal queues which may only accept trusted events.
func ServerOnlyFilter(
	config_obj *config_proto.Config,
	opts services.JournalOptions,
	watcher_name string,
	event *ordereddict.Dict) bool {
	username, pres := event.GetString("_Source")
	if pres && username == constants.VELOCIRAPTOR_SERVER_CLIENT_ID {
		return true
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Error("<red>Untrusted event for %v</> %v for %v sent from %v",
		watcher_name, event, opts.ArtifactName, username)
	return false
}

// The following are hard coded internal message queues.
var (
	// A noop filter that allows all events.
	AllowAllFilter func(
		config_obj *config_proto.Config,
		opts services.JournalOptions,
		watcher_name string,
		event *ordereddict.Dict) bool = nil

	// Used to receive alerts from various places.
	ALERT_QUEUE = services.JournalOptions{
		ArtifactName: "Server.Internal.Alerts",
		ArtifactType: artifact_modes.MODE_SERVER_EVENT,
		ClientId:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
	}

	// Get notified when the artifact definition is updated. Only
	// servers or minions may send this.

	// NOTE: artifacts may be modified by the GUI (which is sending
	// this message to the minions) or by the minions (who might be
	// running the artifact_set() function in a VQL notebook)
	ARTIFACT_MODIFICATION = services.JournalOptions{
		ArtifactName: "Server.Internal.ArtifactModification",
		ArtifactType: artifact_modes.MODE_SERVER_EVENT,
		ClientId:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	// Notify that a client changed label
	LABEL_QUEUE = services.JournalOptions{
		ArtifactName: "Server.Internal.Label",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	TIMELINE_ADD = services.JournalOptions{
		ArtifactName: "Server.Internal.TimelineAdd",
		ArtifactType: artifact_modes.MODE_SERVER_EVENT,
		EventFilter:  AllowAllFilter,
	}

	FLOW_COMPLETION = services.JournalOptions{
		ArtifactName: "System.Flow.Completion",
		ArtifactType: artifact_modes.MODE_CLIENT_EVENT,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,

		// Allow users to re-issue flow completions to trigger post
		// processing steps.
		EventFilter: AllowAllFilter,
	}

	// Emitted when an upload is completed.
	UPLOAD_COMPLETION = services.JournalOptions{
		ArtifactName: "System.Upload.Completion",
		ArtifactType: artifact_modes.MODE_CLIENT_EVENT,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,

		// Allow users to re-issue flow completions to trigger post
		// processing steps.
		EventFilter: AllowAllFilter,
	}

	// Notify when a new client is fully interrogated and ready to
	// work on. The typicaly flow for a new client:
	// 1. Enroll - we have keys but do not know things like hostname etc.
	// 2. Interrogate - We collect basic info and send an event on
	//    INTERROGATION_QUEUE
	// 3. Client is ready for use - watch this queue
	INTERROGATION_QUEUE = services.JournalOptions{
		ArtifactName: "Server.Internal.Interrogation",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,

		// This event is used by VQL to create automations.
		EventFilter: AllowAllFilter,
	}

	ENROLLMENT_QUEUE = services.JournalOptions{
		ArtifactName: "Server.Internal.Enrollment",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,

		// Only triggered internally.
		EventFilter: ServerOnlyFilter,
	}

	// This provides a way for users to listen to these events. The
	// server does not watch this queue.
	CLIENT_CONFLICT = services.JournalOptions{
		ArtifactName: "Server.Internal.ClientConflict",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  AllowAllFilter,
	}

	// Keepalive messages between master and minions
	PING = services.JournalOptions{
		ArtifactName: "Server.Internal.Ping",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	PONG = services.JournalOptions{
		ArtifactName: "Server.Internal.Pong",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	// Triggers Notifications
	NOTIFICATION_QUEUE = services.JournalOptions{
		ArtifactName: "Server.Internal.Notifications",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		Sync:         true,
		EventFilter:  ServerOnlyFilter,
	}

	// Inform minions that the client is added to the hunt
	HUNT_PARTICIPATION = services.JournalOptions{
		ArtifactName: "System.Hunt.Participation",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	HUNT_UPDATE = services.JournalOptions{
		ArtifactName: "Server.Internal.HuntUpdate",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	HUNT_CREATION = services.JournalOptions{
		ArtifactName: "System.Hunt.Creation",
		ArtifactType: artifact_modes.MODE_SERVER_EVENT,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,

		// Used by VQL automation to be notified when a hunt is created.
		EventFilter: AllowAllFilter,
	}

	// Notify minions that the hunt was modified via a mutation.
	HUNT_MODIFICATIONS = services.JournalOptions{
		ArtifactName: "Server.Internal.HuntModification",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	// Inform the frontend service of general metrics of this minion
	FRONTEND_METRICS = services.JournalOptions{
		ArtifactName: "Server.Internal.FrontendMetrics",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	HEALTH_STATS = services.JournalOptions{
		ArtifactName: "Server.Monitor.Health/Prometheus",
		ArtifactType: artifact_modes.MODE_SERVER_EVENT,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	// Master sending cliet_info updates to minions
	CLIENT_INFO_SYNC = services.JournalOptions{
		ArtifactName: "Server.Internal.ClientPing",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	CLIENT_DELETE_QUEUE = services.JournalOptions{
		ArtifactName: "Server.Internal.ClientDelete",
		ArtifactType: artifact_modes.MODE_SERVER_EVENT,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	CLIENT_METADATA_MODIFICATION = services.JournalOptions{
		ArtifactName: "Server.Internal.MetadataModifications",
		ArtifactType: artifact_modes.MODE_SERVER_EVENT,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	CLIENT_INFO_SNAPSHOT_READY = services.JournalOptions{
		ArtifactName: "Server.Internal.ClientInfoSnapshot",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	// Queue to inform minions that a client has tasks pending.
	CLIENT_INFO_TASK = services.JournalOptions{
		ArtifactName: "Server.Internal.ClientTasks",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	// Message all the other nodes that these new flows are in
	// flight. The event listener will add them to the client info
	// service.
	CLIENT_INFO_SCHEDULED = services.JournalOptions{
		ArtifactName: "Server.Internal.ClientScheduled",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	INVENTORY_UPDATED = services.JournalOptions{
		ArtifactName: "Server.Internal.Inventory",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	MASTER_REGISTRATIONS = services.JournalOptions{
		ArtifactName: "Server.Internal.MasterRegistrations",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	USER_MANAGER = services.JournalOptions{
		ArtifactName: "Server.Internal.UserManager",
		ArtifactType: artifact_modes.MODE_INTERNAL,
		Username:     constants.VELOCIRAPTOR_SERVER_CLIENT_ID,
		EventFilter:  ServerOnlyFilter,
	}

	// All well known queues.
	WELL_KNOWN_QUEUES = []services.JournalOptions{
		ALERT_QUEUE, ARTIFACT_MODIFICATION, LABEL_QUEUE,
		TIMELINE_ADD, FLOW_COMPLETION, UPLOAD_COMPLETION,
		INTERROGATION_QUEUE, ENROLLMENT_QUEUE, CLIENT_CONFLICT,
		PING, PONG, NOTIFICATION_QUEUE, HUNT_PARTICIPATION,
		HUNT_UPDATE, HUNT_CREATION, HUNT_MODIFICATIONS,
		FRONTEND_METRICS, HEALTH_STATS, CLIENT_INFO_SYNC,
		CLIENT_DELETE_QUEUE, CLIENT_METADATA_MODIFICATION,
		CLIENT_INFO_SNAPSHOT_READY, CLIENT_INFO_TASK,
		CLIENT_INFO_SCHEDULED, INVENTORY_UPDATED,
		MASTER_REGISTRATIONS, USER_MANAGER,
	}

	WELL_KNOWN_QUEUES_MAP = make(map[string]services.JournalOptions)
)

func init() {
	for _, q := range WELL_KNOWN_QUEUES {
		WELL_KNOWN_QUEUES_MAP[q.ArtifactName] = q
	}
}
