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
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	pool_mu sync.Mutex

	// The global client executor which is wrapped by the various pool
	// clients.
	rootClientExecutor *poolClientMux
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
type poolClientMux struct {
	*ClientExecutor

	mu sync.Mutex

	// Get transactions by session id
	transaction_by_session_id map[string]*transaction

	// Get transactions by a unique key for the FlowRequest message.
	transaction_by_flow_key map[string]*transaction

	// A list of all pool clients that feed off us.
	clients []*PoolClientExecutor
}

func newPoolClientMux(ctx context.Context, config_obj *config_proto.Config) (*poolClientMux, error) {
	exe, err := NewClientExecutor(ctx, "C.Root", config_obj)
	if err != nil {
		return nil, err
	}

	self := &poolClientMux{
		ClientExecutor:            exe,
		transaction_by_session_id: make(map[string]*transaction),
		transaction_by_flow_key:   make(map[string]*transaction),
	}

	go func() {
		for msg := range self.ClientExecutor.ReadResponse() {

			// Maybe cache the results in a transaction.
			self.maybeCacheResult(msg)

			// Below are just monitoring messages since regular
			// collections are always cached in a transaction.
			if msg.SessionId != "F.Monitoring" {
				continue
			}

			// Forward all the event messages to all clients.
			snapshot := []*PoolClientExecutor{}
			self.mu.Lock()
			for _, client := range self.clients {
				snapshot = append(snapshot, client)
			}
			self.mu.Unlock()

			for _, client := range snapshot {
				select {
				case <-ctx.Done():
					return
				case client.Outbound <- msg:
				}
			}

		}
	}()

	return self, nil
}

// Inspect the response and transform it if needed. Currently we only
// need to replace the Hostname with the pool client's ID so it
// appears to be a different client.
func maybeTransformResponse(response *actions_proto.VQLResponse, id int) *actions_proto.VQLResponse {
	if response != nil {
		// We need to make the Hostname unique so if the response
		// contains a Hostname we need to transform it. This
		// specifically targets Generic.Client.Info interrogation.
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

			new_hostname := fmt.Sprintf("%s-%d", hostname, id)
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

// Inspect the request and derive a unique session key for it
func (self *poolClientMux) getRequestKey(req *crypto_proto.FlowRequest) string {
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

// Compare messages from the real client executor against the cached
// transactions and add them to the transactions. If we detect the
// flow is complete we mark the transactions as done and other clients
// may replay it.
func (self *poolClientMux) maybeCacheResult(response *crypto_proto.VeloMessage) {
	self.mu.Lock()
	defer self.mu.Unlock()

	session_id := response.SessionId

	// Check if the transaction is tracked
	tran, pres := self.transaction_by_session_id[session_id]
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

// Gets the transaction for this request or create a new transaction.
func (self *poolClientMux) getCompletedTransaction(
	ctx context.Context, message *crypto_proto.VeloMessage) *transaction {
	self.mu.Lock()
	defer self.mu.Unlock()

	// We only cache FlowRequest messages.
	if message.FlowRequest == nil {
		return nil
	}

	key := self.getRequestKey(message.FlowRequest)
	// Do not cache empty queries.
	if key == "" {
		return nil
	}

	result, pres := self.transaction_by_flow_key[key]

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
	self.transaction_by_flow_key[key] = trans
	self.transaction_by_session_id[message.SessionId] = trans

	fmt.Printf("Starting transaction for %v\n", message.SessionId)

	// Delegate the actual request for processing, the transaction
	// will be filled in by maybeCacheResult()
	self.ClientExecutor.ProcessRequest(ctx, message)

	return trans
}

func (self *poolClientMux) maybeUpdateEventTable(
	ctx context.Context, req *crypto_proto.VeloMessage) {
	self.mu.Lock()
	defer self.mu.Unlock()

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
	self.event_manager.UpdateEventTable(
		self.ctx, self.wg,
		self.config_obj,
		self.Outbound, req.UpdateEventTable)
}

type PoolClientExecutor struct {
	delegate  *poolClientMux
	Outbound  chan *crypto_proto.VeloMessage
	id        int
	client_id string
}

func (self *PoolClientExecutor) Nanny() *NannyService {
	return Nanny
}

func (self *PoolClientExecutor) FlowManager() *responder.FlowManager {
	return self.delegate.FlowManager()
}

func (self *PoolClientExecutor) EventManager() *actions.EventTable {
	return self.delegate.EventManager()
}

func (self *PoolClientExecutor) ClientId() string {
	return self.client_id
}

func (self *PoolClientExecutor) SendToServer(message *crypto_proto.VeloMessage) {
	self.Outbound <- message
}

func (self *PoolClientExecutor) GetClientInfo() *actions_proto.ClientInfo {
	result := self.delegate.GetClientInfo()
	result.Hostname = fmt.Sprintf("%v-%d", result.Hostname, self.id)
	result.Fqdn = fmt.Sprintf("%v-%d", result.Fqdn, self.id)

	return result
}

func (self *PoolClientExecutor) ReadResponse() <-chan *crypto_proto.VeloMessage {
	return self.Outbound
}

// Feed a server request to the executor for execution.
func (self *PoolClientExecutor) ProcessRequest(
	ctx context.Context,
	message *crypto_proto.VeloMessage) {

	// Handle FlowStatsRequest specially - we just pretend this client
	// does not support this feature.
	if message.FlowStatsRequest != nil {
		responder.MakeErrorResponse(self.Outbound, message.SessionId,
			"Unsupported in Pool Client")
		return
	}

	if message.UpdateEventTable != nil {
		self.delegate.maybeUpdateEventTable(ctx, message)
		return
	}

	tran := self.delegate.getCompletedTransaction(ctx, message)
	if tran != nil {
		// Wait until the transaction is done.
		<-tran.IsDone

		fmt.Printf("Getting %v responses from cache\n", len(tran.Responses))

		// Replay the transaction into the output channel but swap the
		// session id to be from thie request.
		for _, resp := range tran.Responses {
			// Copy the original response and change it to appear as
			// if it came from this session.
			response := proto.Clone(resp).(*crypto_proto.VeloMessage)
			response.SessionId = message.SessionId
			response.RequestId = message.RequestId
			response.VQLResponse = maybeTransformResponse(resp.VQLResponse, self.id)

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
	self.delegate.ProcessRequest(ctx, message)
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
	config_obj *config_proto.Config, id int) (result *PoolClientExecutor, err error) {

	pool_mu.Lock()
	defer pool_mu.Unlock()

	if rootClientExecutor == nil {
		rootClientExecutor, err = newPoolClientMux(ctx, config_obj)
		if err != nil {
			return nil, err
		}
	}

	result = &PoolClientExecutor{
		delegate:  rootClientExecutor,
		id:        id,
		Outbound:  make(chan *crypto_proto.VeloMessage, 10),
		client_id: client_id,
	}

	rootClientExecutor.mu.Lock()
	rootClientExecutor.clients = append(rootClientExecutor.clients, result)
	rootClientExecutor.mu.Unlock()

	return result, nil
}

// Detect if this is a FlowStats message which represents the flow is
// compelte.
func isFlowComplete(message *crypto_proto.VeloMessage) bool {
	if message.FlowStats == nil {
		return false
	}
	return message.FlowStats.FlowComplete
}
