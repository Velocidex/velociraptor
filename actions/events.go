package actions

import (
	"context"
	"log"
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/events"
	"www.velocidex.com/golang/velociraptor/responder"
)

type UpdateEventTable struct{}

func (self *UpdateEventTable) Run(
	config *config.Config,
	ctx context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)
	arg, pres := responder.GetArgs().(*actions_proto.VQLEventTable)
	if !pres {
		responder.RaiseError("Request should be of type VQLEventTable")
		return
	}

	logger := log.New(&LogWriter{responder}, "", log.Lshortfile)

	// Make a new table.
	table, err := events.Update(responder, arg)
	if err != nil {
		logger.Printf("Error updating global event table: %v", err)
	}

	// Make a context for the VQL query.
	new_ctx, cancel := context.WithCancel(context.Background())
	// Cancel the context when the cancel channel is closed.
	go func() {
		select {
		case <-table.Done:
			cancel()
		}
	}()

	// Start a new query for each event.
	action_obj := &VQLClientAction{}

	var wg sync.WaitGroup
	wg.Add(len(table.Events))

	for _, event := range table.Events {
		go func() {
			defer wg.Done()

			action_obj.StartQuery(
				config, new_ctx, responder, event)
		}()
	}

	// Wait here for all queries to finish - this forces the
	// output channel to be open and allows us to write results to
	// the server.
	wg.Wait()
}
