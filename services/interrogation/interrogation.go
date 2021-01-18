/*

  The EnrollmentService is responsible for launching the initial
  interrogation collection on an endpoint when it first appears.

  Velociraptor is a zero registration system - this means when a
  client appears, it provisions its own private key and registeres its
  public key with the server. This enables secure communication with
  the endpoint but we still dont know anything about it!

  The EnrollmentService watches for new clients and schedules the
  Generic.Client.Info artifact on the endpoint. Note that this
  collection is done exactly once the first time we see the client -
  it is likely to become outdated.
*/

package interrogation

import (
	"context"
	"fmt"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type EnrollmentService struct{}

func (self *EnrollmentService) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	events, cancel := journal.Watch(ctx, "Server.Internal.Enrollment")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> Enrollment service.")

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}
				err := self.ProcessRow(ctx, config_obj, event)
				if err != nil {
					logger.Error("Enrollment Service: %v", err)
				}
			}
		}
	}()

	return nil
}

func (self *EnrollmentService) ProcessRow(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {
	client_id, pres := row.GetString("ClientId")
	if !pres {
		return nil
	}

	// Get the client record from the data store.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	client_info := &actions_proto.ClientInfo{}
	client_path_manager := paths.NewClientPathManager(client_id)

	err = db.GetSubject(config_obj, client_path_manager.Path(), client_info)
	if err == nil &&
		// If we have a valid client record we do not need to
		// interrogate. Interrogation happens automatically only once
		// - the first time a client appears.
		client_info.ClientId == client_id ||
		len(client_info.Hostname) > 0 {
		return nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Debug("Interrogating %v", client_id)

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// Issue the flow on the client.
	launcher, err := services.GetLauncher()
	if err != nil {
		return err
	}

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, config_obj, vql_subsystem.NullACLManager{},
		repository,
		&flows_proto.ArtifactCollectorArgs{
			ClientId:  client_id,
			Artifacts: []string{constants.CLIENT_INFO_ARTIFACT},
		})
	if err != nil {
		return err
	}

	// Write an intermediate record while the interrogation is in flight.
	client_info.ClientId = client_id
	client_info.LastInterrogateFlowId = flow_id
	err = db.SetSubject(config_obj, client_path_manager.Path(), client_info)
	if err != nil {
		return err
	}

	// Notify the client
	notifier := services.GetNotifier()
	if notifier != nil {
		err = services.GetNotifier().NotifyListener(config_obj, client_id)
		return err
	}
	return nil
}

// Watch the system's flow completion log for interrogate artifacts.
type InterrogationService struct{}

// Watch for Generic.Client.Info artifacts.
func (self *InterrogationService) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	return watchForFlowCompletion(
		ctx, wg, config_obj, "Generic.Client.Info/BasicInformation",
		func(ctx context.Context, scope vfilter.Scope, row *ordereddict.Dict) {
			err := self.ProcessRow(ctx, config_obj, scope, row)
			if err != nil {
				logger.Error(fmt.Sprintf("InterrogationService: %v", err))
			}
		})
}

func (self *InterrogationService) ProcessRow(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope, row *ordereddict.Dict) error {
	client_id, _ := row.GetString("ClientId")
	if client_id == "" {
		return errors.New("Unknown ClientId")
	}

	flow_id, _ := row.GetString("FlowId")

	path_manager := artifacts.NewArtifactPathManager(config_obj,
		client_id, flow_id, "Generic.Client.Info/BasicInformation")
	row_chan, err := file_store.GetTimeRange(
		ctx, config_obj, path_manager, 0, 0)
	if err != nil {
		return err
	}

	var client_info *actions_proto.ClientInfo
	for row := range row_chan {
		getter := func(field string) string {
			result, _ := row.GetString(field)
			return result
		}

		client_info = &actions_proto.ClientInfo{
			ClientId:              client_id,
			Hostname:              getter("Hostname"),
			System:                getter("OS"),
			Release:               getter("Platform") + getter("PlatformVersion"),
			Architecture:          getter("Architecture"),
			Fqdn:                  getter("Fqdn"),
			ClientName:            getter("Name"),
			ClientVersion:         getter("BuildTime"),
			LastInterrogateFlowId: flow_id,
		}

		label_array, ok := row.GetStrings("Labels")
		if ok {
			client_info.Labels = append(client_info.Labels, label_array...)
		}
	}

	if client_info == nil {
		return errors.New("No Generic.Client.Info results")
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(config_obj,
		client_path_manager.Path(), client_info)
	if err != nil {
		return err
	}

	if len(client_info.Labels) > 0 {
		labeler := services.GetLabeler()
		for _, label := range client_info.Labels {
			err := labeler.SetClientLabel(config_obj, client_id, label)
			if err != nil {
				return err
			}
		}
	}

	// Update the client indexes for the GUI. Add any keywords we
	// wish to be searchable in the UI here.
	keywords := []string{
		"all", // This is used for "." search
		client_id,
		client_info.Hostname,
		client_info.Fqdn,
		"host:" + client_info.Hostname,
	}

	return db.SetIndex(config_obj,
		constants.CLIENT_INDEX_URN,
		client_id, keywords,
	)
}

func StartInterrogationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	enrollment_server := &EnrollmentService{}
	err := enrollment_server.Start(ctx, config_obj, wg)
	if err != nil {
		return err
	}

	result := &InterrogationService{}

	return result.Start(ctx, config_obj, wg)
}
