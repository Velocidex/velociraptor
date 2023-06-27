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

  Once the Generic.Client.Info collection is complete we process the
  results, update indexes etc. When this is done we emit a
  Server.Internal.Interrogation event. Queries that are interested in
  new interrogation results need to watch for
  Server.Internal.Interrogation to ensure they do not race with the
  interrogation service.

*/

package interrogation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/time/rate"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

type EnrollmentService struct {
	limiter *rate.Limiter
}

func (self *EnrollmentService) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Enrollment service for %v.", services.GetOrgName(config_obj))

	// Also watch for customized interrogation artifacts.
	err := journal.WatchForCollectionWithCB(ctx, config_obj, wg,
		"Generic.Client.Info/BasicInformation",
		"InterrogationService",
		func(ctx context.Context,
			config_obj *config_proto.Config,
			client_id, flow_id string) error {
			return self.ProcessInterrogateResults(
				ctx, config_obj, client_id, flow_id,
				"Generic.Client.Info/BasicInformation")
		})
	if err != nil {
		return err
	}

	// Also watch for customized interrogation artifacts.
	err = journal.WatchForCollectionWithCB(ctx, config_obj, wg,
		"Custom.Generic.Client.Info/BasicInformation",
		"InterrogationService",
		func(ctx context.Context,
			config_obj *config_proto.Config,
			client_id, flow_id string) error {
			return self.ProcessInterrogateResults(
				ctx, config_obj, client_id, flow_id,
				"Custom.Generic.Client.Info/BasicInformation")
		})
	if err != nil {
		return err
	}

	return journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Enrollment", "InterrogationService",
		self.ProcessEnrollment)
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
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	client_info, err := client_info_manager.Get(ctx, client_id)

	// If we have a valid client record we do not need to
	// interrogate. Interrogation happens automatically only once
	// - the first time a client appears.
	if err == nil && client_info.LastInterrogateFlowId != "" {
		return nil
	}

	// Wait for rate token
	self.limiter.Wait(ctx)

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	interrogation_artifact := "Generic.Client.Info"

	// Allow the user to override the basic interrogation
	// functionality.  Check for any customized versions
	definition, pres := repository.Get(ctx, config_obj, "Custom.Generic.Client.Info")
	if pres {
		interrogation_artifact = definition.Name
	}

	// Issue the flow on the client.
	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, config_obj, acl_managers.NullACLManager{},
		repository,
		&flows_proto.ArtifactCollectorArgs{
			Creator:   "InterrogationService",
			ClientId:  client_id,
			Artifacts: []string{interrogation_artifact},
		}, func() {
			// Notify the client
			notifier, err := services.GetNotifier(config_obj)
			if err == nil {
				notifier.NotifyListener(ctx,
					config_obj, client_id, "Interrogate")
			}
		})
	if err != nil {
		return err
	}

	// Write an intermediate record while the interrogation is in
	// flight. We are here because the client_info_manager does not
	// have the record in cache, so next Get() will just read it from
	// disk on all minions.
	err = client_info_manager.Set(ctx, &services.ClientInfo{
		actions_proto.ClientInfo{
			ClientId:                    client_id,
			FirstSeenAt:                 uint64(time.Now().Unix()),
			LastInterrogateFlowId:       flow_id,
			LastInterrogateArtifactName: interrogation_artifact,
		}})
	if err != nil {
		return err
	}

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	for _, term := range []string{
		"all", // This is used for "." search
		client_id,
	} {
		err = indexer.SetIndex(client_id, term)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("Unable to set index: %v", err)
		}
	}

	return nil
}

func (self *EnrollmentService) ProcessInterrogateResults(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id, artifact string) error {

	file_store_factory := file_store.GetFileStore(config_obj)
	path_manager, err := artifacts.NewArtifactPathManager(ctx, config_obj,
		client_id, flow_id, artifact)
	if err != nil {
		return err
	}

	rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
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
			ClientId:                    client_id,
			Hostname:                    getter("Hostname"),
			System:                      getter("OS"),
			Release:                     getter("Platform") + getter("PlatformVersion"),
			Architecture:                getter("Architecture"),
			Fqdn:                        getter("Fqdn"),
			ClientName:                  getter("Name"),
			ClientVersion:               getter("Version"),
			BuildUrl:                    getter("build_url"),
			LastInterrogateFlowId:       flow_id,
			LastInterrogateArtifactName: artifact,
		}

		build_time, pres := row.Get("BuildTime")
		if pres {
			t, ok := build_time.(time.Time)
			if ok {
				client_info.BuildTime = t.UTC().Format(time.RFC3339)
			}
		}

		label_array, ok := row.GetStrings("Labels")
		if ok {
			client_info.Labels = append(client_info.Labels, label_array...)
		}

		mac_addresses, ok := row.GetStrings("MACAddresses")
		if ok {
			client_info.MacAddresses = append(
				client_info.MacAddresses, mac_addresses...)
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

	public_key_info := &crypto_proto.PublicKey{}
	err = db.GetSubject(config_obj, client_path_manager.Key(),
		public_key_info)
	if err != nil {
		// Offline clients do not have public key files, so this is
		// not actually an error. In that case FirstSeenAt becomes 0.
	}
	client_info.FirstSeenAt = public_key_info.EnrollTime

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubjectWithCompletion(config_obj,
		client_path_manager.Path(), client_info,

		// Completion
		func() {
			journal.PushRowsToArtifactAsync(ctx, config_obj,
				ordereddict.NewDict().
					Set("ClientId", client_id),
				"Server.Internal.Interrogation")
		})
	if err != nil {
		return err
	}

	// Set labels in the labeler.
	if len(client_info.Labels) > 0 {
		labeler := services.GetLabeler(config_obj)
		for _, label := range client_info.Labels {
			err := labeler.SetClientLabel(ctx, config_obj, client_id, label)
			if err != nil {
				return err
			}
		}
	}

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	// Add MAC addresses to the index.
	for _, mac := range client_info.MacAddresses {
		err := indexer.SetIndex(client_id, "mac:"+mac)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("Unable to set index: %v", err)
		}
	}

	// Update the client indexes for the GUI. Add any keywords we
	// wish to be searchable in the UI here.
	for _, term := range []string{
		"all",
		client_id,
		"host:" + client_info.Fqdn,
		"host:" + client_info.Hostname} {
		err := indexer.SetIndex(client_id, term)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("Unable to set index: %v", err)
		}
	}

	return nil
}

func NewInterrogationService(
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
