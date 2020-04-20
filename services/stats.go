// Collect runtime statistics by running VQL Queries periodically.

package services

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	one_day_active = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stats_client_one_day_actives",
		Help: "Number of one day active clients.",
	}, []string{
		"version",
	})
	seven_day_active = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stats_client_seven_day_actives",
		Help: "Number of 7 day active clients.",
	}, []string{
		"version",
	})
)

type StatsCollector struct {
	config_obj *config_proto.Config
}

func (self *StatsCollector) Start(
	ctx context.Context,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting Stats Collector Service.")

	scope := artifacts.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
		Logger: logging.NewPlainLogger(self.config_obj,
			&logging.FrontendComponent),
	}.Build()
	defer scope.Close()

	// Make sure we do not consume too many resources, these stats
	// are not very important. Rate limit to 10 clients per second.
	vfilter.InstallThrottler(scope, vfilter.NewTimeThrottler(float64(10)))

	collect_stats := func(gauge *prometheus.GaugeVec,
		vql *vfilter.VQL, scope *vfilter.Scope) {
		defer wg.Done()

		for row := range vql.Eval(ctx, scope) {
			count_any, _ := scope.Associative(row, "Count")
			version_any, _ := scope.Associative(row, "Version")
			gauge.WithLabelValues(version_any.(string)).
				Set(float64(count_any.(uint64)))

			select {
			case <-ctx.Done():
				return

			case <-time.After(60 * time.Second):
				break
			}
		}
	}

	vql, _ := vfilter.Parse("SELECT count(items=client_id) AS Count, " +
		"agent_information.version AS Version FROM clients() " +
		"WHERE last_seen_at / 1000000 > now() - 60 * 60 * 24 group by Version")
	wg.Add(1)
	go collect_stats(one_day_active, vql, scope)

	vql, _ = vfilter.Parse("SELECT count(items=client_id) AS Count, " +
		"agent_information.version AS Version FROM clients() " +
		"WHERE last_seen_at / 1000000 > now() - 60 * 60 * 24 * 7 group by Version")

	wg.Add(1)
	go collect_stats(seven_day_active, vql, scope)

	return nil
}

func startStatsCollector(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	result := &StatsCollector{
		config_obj: config_obj,
	}
	return result.Start(ctx, wg)
}
