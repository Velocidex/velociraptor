/*
  Tracking the flow client side.


  A flow is started on the client using the FlowRequest message. This
  message contains multiple VQLClientAction requests - one for each
  query that needs to run in parallel.

* FlowManager is a stateful service which maintains in flight flows on
  the client. This enabled cancellations. The FlowManager service is a
  factory for the FlowContext object

* FlowContext is an object that manages the flow state on the client:
  1. Maintains a batch of log messages.
  2. Maintains a list of FlowResponder objects

* FlowResponder is an object that tracks a single query.
  1. Tracks the query stats (number of rows etc).
  2. Maintain periodic progress messages to send to the server

*/

package responder

import (
	"context"

	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type Responder interface {
	AddResponse(message *crypto_proto.VeloMessage)
	RaiseError(ctx context.Context, message string)
	Return(ctx context.Context)
	Log(ctx context.Context, level string, msg string)
	NextUploadId() int64
	FlowContext() *FlowContext
	Close()
}
