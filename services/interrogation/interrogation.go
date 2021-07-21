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
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type EnrollmentService struct {
	limiter *rate.Limiter
}

func (self *EnrollmentService) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Enrollment service.")

	err := journal.WatchForCollectionWithCB(ctx, config_obj, wg,
		"Generic.Client.Info/BasicInformation",
		self.ProcessInterrogateResults)
	if err != nil {
		return err
	}

	return journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Enrollment", self.ProcessEnrollment)
}

func (self *EnrollmentService) ProcessEnrollment(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {
	client_id, pres := row.GetString("ClientId")
	if !pres {
		return nil
	}

	// Get the client info from the client info manager.
	client_info_manager := services.GetClientInfoManager()
	if client_info_manager == nil {
		return errors.New("Client info manager not started")
	}
	_, err := client_info_manager.Get(client_id)

	// If we have a valid client record we do not need to
	// interrogate. Interrogation happens automatically only once
	// - the first time a client appears.
	if err == nil {
		return nil
	}

	// Wait for rate token
	self.limiter.Wait(ctx)

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
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	client_info := &actions_proto.ClientInfo{
		ClientId:              client_id,
		LastInterrogateFlowId: flow_id,
	}
	err = db.SetSubject(config_obj, client_path_manager.Path(), client_info)
	if err != nil {
		return err
	}

	keywords := []string{
		"all", // This is used for "." search
		client_id,
	}

	err = db.SetIndex(config_obj,
		constants.CLIENT_INDEX_URN,
		client_id, keywords)
	if err != nil {
		return err
	}

	// Notify the client
	notifier := services.GetNotifier()
	if notifier != nil {
		return services.GetNotifier().
			NotifyListener(config_obj, client_id)
	}
	return nil
}

func (self *EnrollmentService) ProcessInterrogateResults(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) error {

	file_store_factory := file_store.GetFileStore(config_obj)
	path_manager, err := artifacts.NewArtifactPathManager(config_obj,
		client_id, flow_id, "Generic.Client.Info/BasicInformation")
	if err != nil {
		return err
	}

	rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager)
	if err != nil {
		return err
	}
	defer rs_reader.Close()

	var client_info *actions_proto.ClientInfo

	// Should return only one row
	for row := range rs_reader.Rows(ctx) {
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
		break
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
		client_info.Hostname,
		client_info.Fqdn,
		"host:" + client_info.Hostname,
	}

	return db.SetIndex(config_obj,
		constants.CLIENT_INDEX_URN,
		client_id, keywords)
}

func StartInterrogationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	limit_rate := config_obj.Frontend.Resources.EnrollmentsPerSecond
	if limit_rate == 0 {
		limit_rate = 100
	}

	// Negative enrollment rate means to disable enrollment service.
	if limit_rate < 0 {
		return nil
	}

	enrollment_server := &EnrollmentService{
		limiter: rate.NewLimiter(rate.Limit(limit_rate), 1),
	}
	return enrollment_server.Start(ctx, config_obj, wg)
}
