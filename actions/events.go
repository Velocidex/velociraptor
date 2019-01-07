package actions

import (
	"context"
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/events"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
)

type UpdateEventTable struct{}

func (self *UpdateEventTable) Run(
	config *api_proto.Config,
	ctx context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)
	arg, pres := responder.GetArgs().(*actions_proto.VQLEventTable)
	if !pres {
		responder.RaiseError("Request should be of type VQLEventTable")
		return
	}

	// Make a new table.
	table, err := events.Update(responder, arg)
	if err != nil {
		responder.Log("Error updating global event table: %v", err)
	}

	// Make a context for the VQL query.
	new_ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context when the cancel channel is closed.
	go func() {
		<-table.Done
		cancel()
	}()

	logger := logging.GetLogger(config, &logging.ClientComponent)

	// Start a new query for each event.
	action_obj := &VQLClientAction{}
	var wg sync.WaitGroup
	wg.Add(len(table.Events))

	for _, event := range table.Events {
		go func(event *actions_proto.VQLCollectorArgs) {
			defer wg.Done()

			name := ""
			for _, q := range event.Query {
				if q.Name != "" {
					name = q.Name
				}
			}

			logger.Info("Starting %s", name)
			action_obj.StartQuery(
				config, new_ctx, responder, event)

			logger.Info("Finished %s", name)
		}(event)
	}

	// Return an OK status. This is needed to make sure the
	// request is de-queued.
	responder.Return()

	// Wait here for all queries to finish - this forces the
	// output channel to be open and allows us to write results to
	// the server.
	wg.Wait()
}
