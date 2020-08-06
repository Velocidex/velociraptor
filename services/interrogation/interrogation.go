package interrogation

import (
	"context"
	"fmt"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/vfilter"
)

type EnrollmentService struct {
	mu sync.Mutex

	APIClientFactory grpc_client.APIClientFactory
	config_obj       *config_proto.Config
	cancel           func()
}

func (self *EnrollmentService) Start(
	ctx context.Context,
	wg *sync.WaitGroup) error {

	// Wait in this func until we are ready to monitor.
	local_wg := &sync.WaitGroup{}
	local_wg.Add(1)

	wg.Add(1)
	go func() {
		defer wg.Done()

		self.mu.Lock()
		defer self.mu.Unlock()

		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Info("Starting Enrollment service.")

		events, cancel := services.GetJournal().Watch("Server.Internal.Enrollment")
		defer cancel()

		local_wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}
				err := self.ProcessRow(ctx, event)
				if err != nil {
					logger.Error("Enrollment Service: %v", err)
				}
			}
		}
	}()

	local_wg.Wait()

	return nil
}

func (self *EnrollmentService) ProcessRow(
	ctx context.Context,
	row *ordereddict.Dict) error {
	client_id, pres := row.GetString("ClientId")
	if !pres {
		return nil
	}

	// Get the client record from the data store.
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	client_info := &actions_proto.ClientInfo{}
	client_path_manager := paths.NewClientPathManager(client_id)

	db.GetSubject(self.config_obj, client_path_manager.Path(), client_info)

	// If we have a valid client record we do not need to
	// interrogate. Interrogation happens automatically only once
	// - the first time a client appears.
	if client_info.ClientId == client_id ||
		len(client_info.Hostname) > 0 {
		return nil
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("Interrogating %v", client_id)

	repository, err := artifacts.GetGlobalRepository(self.config_obj)
	if err != nil {
		return err
	}

	// Issue the flow on the client.
	flow_id, err := services.GetLauncher().ScheduleArtifactCollection(
		ctx, self.config_obj,
		self.config_obj.Client.PinnedServerName, /* principal */
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
	err = db.SetSubject(self.config_obj, client_path_manager.Path(), client_info)

	return err
}

// Watch the system's flow completion log for interrogate artifacts.
type InterrogationService struct {
	mu sync.Mutex

	APIClientFactory grpc_client.APIClientFactory
	config_obj       *config_proto.Config
	cancel           func()
}

// Watch for Generic.Client.Info artifacts.
func (self *InterrogationService) Start(
	ctx context.Context,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	watchForFlowCompletion(
		ctx, wg, self.config_obj, "Generic.Client.Info/BasicInformation",
		func(ctx context.Context, scope *vfilter.Scope, row *ordereddict.Dict) {
			err := self.ProcessRow(ctx, scope, row)
			if err != nil {
				logger.Error(fmt.Sprintf("InterrogationService: %v", err))
			}
		})

	return nil
}

func (self *InterrogationService) ProcessRow(
	ctx context.Context, scope *vfilter.Scope, row *ordereddict.Dict) error {
	client_id, _ := row.GetString("ClientId")
	if client_id == "" {
		return errors.New("Unknown ClientId")
	}

	flow_id, _ := row.GetString("FlowId")

	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		client_id, flow_id, "Generic.Client.Info/BasicInformation")
	row_chan, err := file_store.GetTimeRange(
		ctx, self.config_obj, path_manager, 0, 0)
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
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(self.config_obj,
		client_path_manager.Path(), client_info)
	if err != nil {
		return err
	}

	if len(client_info.Labels) > 0 {
		labeler := services.GetLabeler()
		for _, label := range client_info.Labels {
			labeler.SetClientLabel(client_id, label)
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

	return db.SetIndex(self.config_obj,
		constants.CLIENT_INDEX_URN,
		client_id, keywords,
	)
}

func StartInterrogationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	enrollment_server := &EnrollmentService{
		config_obj:       config_obj,
		APIClientFactory: grpc_client.GRPCAPIClient{},
	}
	enrollment_server.Start(ctx, wg)

	result := &InterrogationService{
		config_obj:       config_obj,
		APIClientFactory: grpc_client.GRPCAPIClient{},
	}

	return result.Start(ctx, wg)
}
