/*
   The pool client pretends to be a large number of clients in order
   to exert a large load on the server.  In reality each client is
   running in a go routine in parallel.

   Therefore when we do a hunt, each pool client goroutine will
   receive the same VQL query and run the same code. This reduces the
   load the pool client can impart since it is busy running the same
   query multiple times.

   This pool executor memoizes the results from each query in memory
   so each query is run only once but the results are returned from
   each goroutine fake client as if it was unique. This increases the
   total number of pool clients we can support since most of the work
   is pushed out to the comms.
*/

package executor

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	pool_mu sync.Mutex

	// Get transactions by session id
	transaction_by_session_id = make(map[string]*transaction)

	// Get transactions by a unique key for the FlowRequest message.
	transaction_by_flow_key = make(map[string]*transaction)
)

type transaction struct {
	Request   *crypto_proto.VeloMessage
	Responses []*crypto_proto.VeloMessage

	// While the transaction is running we need to make other
	// threads wait until it is done.
	IsDone chan bool
	Done   bool
}

// A wrapper around the standard client executor for use of pool
// clients. When multiple requests come in for the same query and
// parameters, we cache the results when the first request comes in
// and then feed the results to all other requests from memory. This
// allows us to increase the load on the server simulating a large
// fleet of independent clients.
type PoolClientExecutor struct {
	*ClientExecutor
	Outbound chan *crypto_proto.VeloMessage
	id       int
}

func (self *PoolClientExecutor) ReadResponse() <-chan *crypto_proto.VeloMessage {
	return self.Outbound
}

// Inspect the request and derive a unique session key for it
func getRequestKey(req *crypto_proto.FlowRequest) string {
	key := ""

	for _, action := range req.VQLClientActions {
		for _, query := range action.Query {
			key += query.Name
		}

		// Cache it under the query name and the serialized
		// parameters. This way when any of the parameters change we
		// recalculate the query.
		key += json.MustMarshalString(action.Env)
	}
	return key
}

// Gets the transaction for this request or create a new transaction.
func getCompletedTransaction(message *crypto_proto.VeloMessage) *transaction {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	// We only cache FlowRequest messages.
	if message.FlowRequest == nil {
		return nil
	}

	key := getRequestKey(message.FlowRequest)
	// Do not cache empty queries.
	if key == "" {
		return nil
	}

	result, pres := transaction_by_flow_key[key]

	// Transaction fully cached and completed.
	if pres {
		return result
	}

	// There is no transaction there yet so build one ready for
	// the results.
	trans := &transaction{
		Request: message,
		IsDone:  make(chan bool),
	}

	// Cache it for the next
	transaction_by_flow_key[key] = trans
	transaction_by_session_id[message.SessionId] = trans

	fmt.Printf("Starting transaction for %v\n", message.SessionId)

	// Return nil for the next caller to start executing this transaction.
	return nil
}

func (self *PoolClientExecutor) maybeUpdateEventTable(
	ctx context.Context, req *crypto_proto.VeloMessage) {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	// Only update newer tables.
	if req.UpdateEventTable.Version <= self.event_manager.Version() {
		return
	}

	// In practice each client receives its own event table
	// version which is the timestamp of the last table update. In
	// the pool client we do not want to refresh the table too
	// much so we set the version far into the future. This means
	// that it is impossible to update the pool client's event
	// table without a restart.
	req.UpdateEventTable.Version += 6000 * 1000000000

	g_responder := responder.GetPoolEventResponder(ctx)
	self.event_manager.UpdateEventTable(
		self.ctx, self.wg,
		self.config_obj,
		g_responder.EventTableInput, req.UpdateEventTable)

}

// Feed a server request to the executor for execution.
func (self *PoolClientExecutor) ProcessRequest(
	ctx context.Context,
	message *crypto_proto.VeloMessage) {

	if message.UpdateEventTable != nil {
		self.maybeUpdateEventTable(ctx, message)
		return
	}

	tran := getCompletedTransaction(message)
	if tran != nil {
		// Wait until the transaction is done.
		<-tran.IsDone

		fmt.Printf("Getting %v responses from cache\n", len(tran.Responses))

		// Replay the transaction into the output channel.
		for _, resp := range tran.Responses {
			response := &crypto_proto.VeloMessage{
				SessionId:   message.SessionId,
				RequestId:   message.RequestId,
				VQLResponse: self.maybeTransformResponse(resp.VQLResponse),
				LogMessage:  resp.LogMessage,
				FlowStats:   resp.FlowStats,
			}
			select {
			case <-ctx.Done():
				return
			case self.Outbound <- response:
			}
		}
		return
	}

	// If we get here there is no cached transaction - just forward to
	// the normal executor.
	self.ClientExecutor.ProcessRequest(ctx, message)
}

// Inspect the response and transform it if needed. Currently we only
// need to replace the Hostname with the pool client's ID so it
// appears to be a different client.
func (self *PoolClientExecutor) maybeTransformResponse(
	response *actions_proto.VQLResponse) *actions_proto.VQLResponse {

	if response != nil {

		// We need to make the Hostname unique so if the response contains
		// a Hostname we need to transform it.
		if utils.InString(response.Columns, "Hostname") {
			rows, err := utils.ParseJsonToDicts([]byte(response.JSONLResponse))
			if err != nil || len(rows) == 0 {
				return response
			}

			// Replace the Hostname
			hostname, pres := rows[0].Get("Hostname")
			if !pres {
				return response
			}

			new_hostname := fmt.Sprintf("%s-%d", hostname, self.id)
			rows[0].Set("Fqdn", new_hostname)
			rows[0].Set("Hostname", new_hostname)

			new_rows, err := json.MarshalJsonl(rows)
			if err != nil {
				return response
			}
			result := proto.Clone(response).(*actions_proto.VQLResponse)
			result.JSONLResponse = string(new_rows)

			return result
		}
	}
	return response
}

// A Pool Client is a virtualized client running in a goroutine which
// emulates a full blown client. Flow Requests are cached globally in
// a transaction so they can be replayed back for all clients. This
// allows us to calculate any query once but return all the results at
// once from other clients immediately therefore increasing the load
// on the server.
func NewPoolClientExecutor(
	ctx context.Context,
	client_id string,
	config_obj *config_proto.Config, id int) (*PoolClientExecutor, error) {
	exe, err := NewClientExecutor(ctx, client_id, config_obj)
	if err != nil {
		return nil, err
	}

	// Register the new executor with the global pool responder.
	g_responder := responder.GetPoolEventResponder(ctx)
	g_responder.RegisterPoolClientResponder(id, exe.Outbound)

	output := make(chan *crypto_proto.VeloMessage, 10)

	go func() {
		delegate_messages := exe.ReadResponse()
		for {
			select {
			case <-ctx.Done():
				return

			case message := <-delegate_messages:
				if message.SessionId != "F.Monitoring" {
					maybeCacheResult(message)
				}
				output <- message
			}
		}
	}()

	return &PoolClientExecutor{
		ClientExecutor: exe,
		Outbound:       output,
		id:             id,
	}, nil
}

// Compare messages from the real client executor against the cached
// transactions and add them to the transactions. If we detect the
// flow is complete we mark the transactions as done and other clients
// may replay it.
func maybeCacheResult(response *crypto_proto.VeloMessage) {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	session_id := response.SessionId

	// Check if the transaction is tracked
	tran, pres := transaction_by_session_id[session_id]
	if pres {
		fmt.Printf("%v\n", response)
		tran.Responses = append(tran.Responses, response)

		// Determine if the flow is completed by looking at the FlowStat
		if !tran.Done && isFlowComplete(response) {
			fmt.Printf("Completing transaction for session_id %v\n",
				session_id)
			// The transaction is now done.
			close(tran.IsDone)
			tran.Done = true
		}
	}
}

// Detect if this is a FlowStats message which represents the flow is
// compelte.
func isFlowComplete(message *crypto_proto.VeloMessage) bool {
	if message.FlowStats == nil {
		return false
	}

	for _, s := range message.FlowStats.QueryStatus {
		// Flow is not completed as one of the queries is still
		// running.
		if s.Status == crypto_proto.VeloStatus_PROGRESS {
			return false
		}
	}
	return true
}
