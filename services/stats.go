// Collect runtime statistics by running VQL Queries periodically.

package services

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
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
	done       chan bool
}

func (self *StatsCollector) Start() error {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting Stats Collector Service.")

	env := vfilter.NewDict().
		Set("config", self.config_obj.Client).
		Set("server_config", self.config_obj)

	repository, err := artifacts.GetGlobalRepository(self.config_obj)
	if err != nil {
		return err
	}
	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(self.config_obj,
		&logging.FrontendComponent)

	// Make sure we do not consume too many resources, these stats
	// are not very important. Rate limit to 10 clients per second.
	vfilter.InstallThrottler(scope, vfilter.NewTimeThrottler(float64(10)))

	collect_stats := func(gauge *prometheus.GaugeVec, vql *vfilter.VQL, scope *vfilter.Scope) {
		for {
			row_chan := vql.Eval(context.Background(), scope)
		run_query:
			for {
				select {
				case <-self.done:
					return

				case row, ok := <-row_chan:
					if !ok {
						break run_query
					}
					count_any, _ := scope.Associative(row, "Count")
					version_any, _ := scope.Associative(row, "Version")
					gauge.WithLabelValues(version_any.(string)).
						Set(float64(count_any.(uint64)))
				}
			}

			time.Sleep(60 * time.Second)
		}
	}

	vql, _ := vfilter.Parse("SELECT count(items=client_id) AS Count, " +
		"agent_information.version AS Version FROM clients() " +
		"WHERE last_seen_at / 1000000 > now() - 60 * 60 * 24 group by Version")
	go collect_stats(one_day_active, vql, scope)

	vql, _ = vfilter.Parse("SELECT count(items=client_id) AS Count, " +
		"agent_information.version AS Version FROM clients() " +
		"WHERE last_seen_at / 1000000 > now() - 60 * 60 * 24 * 7 group by Version")
	go collect_stats(seven_day_active, vql, scope)

	return nil
}

func (self *StatsCollector) Close() {
	close(self.done)
}

func startStatsCollector(config_obj *config_proto.Config) (*StatsCollector, error) {
	result := &StatsCollector{
		config_obj: config_obj,
		done:       make(chan bool),
	}

	err := result.Start()
	return result, err
}
