package flows

import (
	"context"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
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
func checkContextResourceLimits(
	ctx context.Context,
	config_obj *config_proto.Config,
	collection_context *CollectionContext) (err error) {

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
		err = cancelCollection(
			ctx, config_obj, collection_context.ClientId,
			collection_context.SessionId)
	}

	// Check for total uploaded bytes.
	if collection_context.Request.MaxUploadBytes > 0 &&
		collection_context.TotalUploadedBytes > collection_context.Request.MaxUploadBytes {
		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = "Collection exceeded upload limits"
		err = cancelCollection(
			ctx, config_obj, collection_context.ClientId,
			collection_context.SessionId)
	}

	return err
}

func cancelCollection(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) error {
	// Cancel the collection to stop the client from generating
	// more data.
	client_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	return client_manager.QueueMessageForClient(ctx, client_id,
		&crypto_proto.VeloMessage{
			Cancel:    &crypto_proto.Cancel{},
			SessionId: flow_id,
		},
		services.NOTIFY_CLIENT, utils.BackgroundWriter)
}
