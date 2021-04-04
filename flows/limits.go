package flows

import (
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	rowCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "received_rows",
		Help: "Total number of rows received from clients.",
	})
)

// Sometimes it is hard to predict exactly how much data a client is
// going to send. Velociraptor implements a number of limits to
// protect the server from being overloaded.  This function checks
// that the collection is still within the allowed quota, otherwise
// the collection is terminated and the client is notified that it is
// cancelled.
func checkContextResourceLimits(config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext) (err error) {

	// There are no resource limits on event flows.
	if collection_context.SessionId == constants.MONITORING_WELL_KNOWN_FLOW {
		return nil
	}

	if collection_context.Request == nil {
		return errors.New("Invalid context.")
	}

	// We exceeded our total number of rows.
	if collection_context.Request.MaxRows > 0 &&
		collection_context.TotalCollectedRows > collection_context.Request.MaxRows {
		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = "Row count exceeded limit"
		err = cancelCollection(config_obj, collection_context.ClientId,
			collection_context.SessionId)
	}

	// Check for total uploaded bytes.
	if collection_context.Request.MaxUploadBytes > 0 &&
		collection_context.TotalUploadedBytes > collection_context.Request.MaxUploadBytes {
		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = "Collection exceeded upload limits"
		err = cancelCollection(config_obj, collection_context.ClientId,
			collection_context.SessionId)
	}

	return err
}

func cancelCollection(config_obj *config_proto.Config, client_id, flow_id string) error {
	// Cancel the collection to stop the client from generating
	// more data.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	err = db.QueueMessageForClient(config_obj, client_id,
		&crypto_proto.VeloMessage{
			Cancel:    &crypto_proto.Cancel{},
			SessionId: flow_id,
		})
	if err != nil {
		return err
	}

	// Notify the client immediately.
	return services.GetNotifier().NotifyListener(config_obj, client_id)
}
