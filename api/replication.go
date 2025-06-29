package api

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/vfilter"
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

	gReplicationTracker = &replicationTracker{
		currentReplications: make(map[string]*replicatedStats),
	}
)

func streamEvents(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.EventRequest,
	stream api_proto.API_WatchEventServer,
	peer_name string,
	stats *replicatedStats) (err error) {

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
		err := stream.Send(&api_proto.EventResponse{
			Jsonl: serialized,
		})
		if err != nil {
			return err
		}
		stats.Sent++
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

		case event, ok := <-output_chan:
			if !ok {
				return
			}

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

			// If we are not able to send within the sepecified 5
			// seconds we must abort the connection.

			err = utils.DoWithTimeout(func() error {
				return stream.Send(response)
			}, 5*time.Second)
			if err != nil {
				return err
			}

			timer.ObserveDuration()
			stats.Sent++

			if err != nil {
				continue
			}
		}
	}
}

// NOTE: The API server is only running on the master node.
func (self *ApiServer) WatchEvent(
	in *api_proto.EventRequest,
	stream api_proto.API_WatchEventServer) error {

	// Get the TLS context from the peer and verify its
	// certificate.
	ctx := stream.Context()
	users := services.GetUserManager()
	user_record, config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return err
	}

	// This name is taken from the certificate usually
	// VelociraptorServer.
	peer_name := user_record.Name

	// Check that the principal is allowed to issue queries.
	permissions := acls.ANY_QUERY
	ok, err := services.CheckAccess(config_obj, peer_name, permissions)
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

	// Update the peer name to make it unique
	peer_addr, ok := peer.FromContext(ctx)
	if ok {
		peer_name = strings.Split(peer_addr.Addr.String(), ":")[0]
	}

	// Wait here for orderly shutdown of event streams.
	self.wg.Add(1)
	defer self.wg.Done()

	// The call can access the datastore from any org becuase it is a
	// server->server call.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	org_config_obj, err := org_manager.GetOrgConfig(in.OrgId)
	if err != nil {
		return err
	}

	// Cert is good enough for us, run the query.
	stats, closer := gReplicationTracker.Add(in.Queue, peer_name, in.OrgId)
	defer closer()

	return streamEvents(
		ctx, org_config_obj, in, stream, peer_name, stats)
}

type replicatedStats struct {
	Sent int
}

type replicationTracker struct {
	mu                  sync.Mutex
	currentReplications map[string]*replicatedStats
}

func (self *replicationTracker) Debug() []*ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []*ordereddict.Dict{}
	keys := []string{}
	for k := range self.currentReplications {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, k := range keys {
		v, _ := self.currentReplications[k]
		result = append(result, ordereddict.NewDict().
			Set("Type", "Replication").
			Set("Name", k).
			Set("Stats", v))
	}
	return result
}

func (self *replicationTracker) Add(queue, peer, org_id string) (*replicatedStats, func()) {
	key := queue + "->" + peer + " " + org_id
	self.mu.Lock()
	defer self.mu.Unlock()

	stats := &replicatedStats{}

	self.currentReplications[key] = stats

	return stats, func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		delete(self.currentReplications, key)
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:        "Replication",
		Description: "Report current replication connections between master and minion",
		Categories:  []string{"Global", "Datastore"},
		ProfileWriter: func(ctx context.Context,
			scope vfilter.Scope, output_chan chan vfilter.Row) {

			for _, i := range gReplicationTracker.Debug() {
				output_chan <- i
			}
		},
	})
}
