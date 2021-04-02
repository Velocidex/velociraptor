package services

import (
	"context"
	"os"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

// The frontend service manages load balancing between multiple
// frontends. Velociraptor clients may be redirected between active
// frontends to spread the load between them.

var (
	frontend_mu sync.Mutex

	gFrontend FrontendManager

	FrontendIsMaster = os.ErrNotExist
)

func RegisterFrontendManager(frontend FrontendManager) {
	frontend_mu.Lock()
	defer frontend_mu.Unlock()

	gFrontend = frontend
}

func GetFrontendManager() FrontendManager {
	frontend_mu.Lock()
	defer frontend_mu.Unlock()

	return gFrontend
}

type FrontendManager interface {
	IsMaster() bool

	// Establish a gRPC connection to the master node. If we are
	// running on the master node already then returns a
	// fs.ErrNotExist error. If we fail to connect returns another
	// error.
	GetMasterAPIClient(ctx context.Context) (
		api_proto.APIClient, func() error, error)
}
