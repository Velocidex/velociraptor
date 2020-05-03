package services

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/clients"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Watch the system's flow completion log for interrogate artifacts.
type InterrogationService struct {
	mu sync.Mutex

	APIClientFactory grpc_client.APIClientFactory
	config_obj       *config_proto.Config
	cancel           func()
}

func (self *InterrogationService) Start(
	ctx context.Context,
	wg *sync.WaitGroup) error {
	defer wg.Done()

	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting interrogation service.")

	scope := artifacts.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
	}.Build()
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(self.config_obj,
		&logging.FrontendComponent)

	vql, _ := vfilter.Parse("SELECT * FROM Artifact.Server.Internal.Interrogate()")
	go func() {
		for row := range vql.Eval(ctx, scope) {
			row_dict := vfilter.RowToDict(ctx, scope, row)
			err := self.ProcessRow(row_dict)
			if err != nil {
				logger.Error("Interrogation Service: %v", err)
			}
		}
	}()

	return nil
}

func (self *InterrogationService) ProcessRow(row *ordereddict.Dict) error {
	client_id, ok := row.GetString("ClientId")
	if !ok {
		return errors.New("Unknown ClientId")
	}

	getter := func(field string) string {
		result, _ := row.GetString(field)
		return result
	}

	client_info := &actions_proto.ClientInfo{
		Hostname:              getter("Hostname"),
		System:                getter("OS"),
		Release:               getter("Platform") + getter("PlatformVersion"),
		Architecture:          getter("Architecture"),
		Fqdn:                  getter("Fqdn"),
		ClientName:            getter("Name"),
		ClientVersion:         getter("BuildTime"),
		LastInterrogateFlowId: getter("FlowId"),
	}

	label_array, ok := row.GetStrings("Labels")
	if ok {
		client_info.Labels = append(client_info.Labels, label_array...)
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
		err := clients.LabelClients(
			self.config_obj,
			&api_proto.LabelClientsRequest{
				ClientIds: []string{client_id},
				Labels:    client_info.Labels,
				Operation: "set",
			})
		if err != nil {
			return err
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

func startInterrogationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) {
	result := &InterrogationService{
		config_obj:       config_obj,
		APIClientFactory: grpc_client.GRPCAPIClient{},
	}

	wg.Add(1)
	go result.Start(ctx, wg)
}
