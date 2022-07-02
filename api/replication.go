package api

import (
	"fmt"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	replicationReceiveHistorgram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "replication_master_send_latency",
			Help:    "Latency for the master to send replication messages to the minion.",
			Buckets: prometheus.LinearBuckets(0.1, 1, 10),
		},
		[]string{"status"},
	)
)

func streamEvents(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.EventRequest,
	stream api_proto.API_WatchEventServer,
	peer_name string) (err error) {

	logger := logging.GetLogger(config_obj, &logging.APICmponent)
	logger.WithFields(logrus.Fields{
		"arg":  in,
		"user": peer_name,
	}).Info("Replicating Events")

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	// Special case this so the caller can immediately initialize the
	// watchers.
	if in.Queue == "Server.Internal.MasterRegistrations" {
		result := ordereddict.NewDict().Set("Events", journal.GetWatchers())
		serialized, _ := result.MarshalJSON()
		stream.Send(&api_proto.EventResponse{
			Jsonl: serialized,
		})
	}

	// The API service is running on the master only! This means
	// the journal service is local.
	output_chan, cancel := journal.Watch(
		ctx, in.Queue, "replication-"+in.WatcherName)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-output_chan:
			serialized, err := json.Marshal(event)
			if err != nil {
				continue
			}
			response := &api_proto.EventResponse{
				Jsonl: serialized,
			}

			timer := prometheus.NewTimer(
				prometheus.ObserverFunc(func(v float64) {
					replicationReceiveHistorgram.WithLabelValues("").Observe(v)
				}))

			err = stream.Send(response)
			timer.ObserveDuration()

			if err != nil {
				continue
			}
		}
	}

	return nil
}

// NOTE: The API server is only running on the master node.
func (self *ApiServer) WatchEvent(
	in *api_proto.EventRequest,
	stream api_proto.API_WatchEventServer) error {

	// Get the TLS context from the peer and verify its
	// certificate.
	ctx := stream.Context()
	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return err
	}

	peer_name := user_record.Name

	// Check that the principal is allowed to issue queries.
	permissions := acls.ANY_QUERY
	ok, err := acls.CheckAccess(org_config_obj, peer_name, permissions)
	if err != nil {
		return status.Error(codes.PermissionDenied,
			fmt.Sprintf("User %v is not allowed to run queries.",
				peer_name))
	}

	if !ok {
		return status.Error(codes.PermissionDenied, fmt.Sprintf(
			"Permission denied: User %v requires permission %v to run queries",
			peer_name, permissions))
	}

	// Wait here for orderly shutdown of event streams.
	self.wg.Add(1)
	defer self.wg.Done()

	// Cert is good enough for us, run the query.
	return streamEvents(
		ctx, org_config_obj, in, stream, peer_name)
}
