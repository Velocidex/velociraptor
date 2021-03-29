package services

import (
	"context"
	"os"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

// The frontend service manages load balancing between multiple
// frontends. Velociraptor clients may be redirected between active
// frontends to spread the load between them.

var (
	Frontend FrontendManager

	FrontendIsMaster = os.ErrNotExist
)

type FrontendManager interface {
	// The FrontendManager returns a URL to an active
	// frontend. The method may be used to redirect a client to an
	// active and ready frontend.
	GetFrontendURL() (string, bool)

	GetNodeName() string
	GetMasterName() string

	// Establish a gRPC connection to the master node. If we are
	// running on the master node already then returns a
	// fs.ErrNotExist error. If we fail to connect returns another
	// error.
	GetMasterAPIClient(ctx context.Context) (
		api_proto.APIClient, func() error, error)
}
