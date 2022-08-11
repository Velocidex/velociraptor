package journal

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Watch "System.Flow.Completion" and trigger when a specific artifact
// is collected.
func WatchForCollectionWithCB(ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	artifact, watcher_name string,

	processor func(ctx context.Context,
		config_obj *config_proto.Config,
		client_id, flow_id string) error) error {

	return WatchQueueWithCB(ctx, config_obj, wg, "System.Flow.Completion",
		watcher_name,
		func(ctx context.Context, config_obj *config_proto.Config,
			row *ordereddict.Dict) error {

			// Extract the flow description from the event.
			flow := &flows_proto.ArtifactCollectorContext{}
			flow_any, _ := row.Get("Flow")
			err := utils.ParseIntoProtobuf(flow_any, flow)
			if err != nil {
				return err
			}

			// This is not what we are looking for.
			if !utils.InString(flow.ArtifactsWithResults, artifact) {
				return nil
			}

			client_id, _ := row.GetString("ClientId")
			if client_id == "" {
				return errors.New("Unknown ClientId")
			}

			flow_id, _ := row.GetString("FlowId")

			return processor(ctx, config_obj, client_id, flow_id)
		})
}

// Watch a queue and apply a processor on any rows received.
func WatchQueueWithCB(ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	artifact, watcher_name string,

	// A processor for rows from the queue
	processor func(ctx context.Context,
		config_obj *config_proto.Config,
		row *ordereddict.Dict) error) error {

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}
	qm_chan, cancel := journal.Watch(ctx, artifact, watcher_name)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			select {
			case row, ok := <-qm_chan:
				if !ok {
					return
				}
				err := processor(ctx, config_obj, row)
				if err != nil {
					// Processor errors are not generally serious so
					// we just log them into the debug log.
					logger := logging.GetLogger(config_obj,
						&logging.FrontendComponent)
					logger.WithFields(logrus.Fields{
						"Owner":    watcher_name,
						"Artifact": artifact,
						"Error":    err.Error(),
					}).Debug("Event Processor Error")
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}
