package services

// The frontend service manages load balancing between multiple
// frontends. Velociraptor clients may be redirected between active
// frontends to spread the load between them.

var (
	Frontend FrontendManager
)

type FrontendManager interface {
	// The FrontendManager returns a URL to an active
	// frontend. The method may be used to redirect a client to an
	// active and ready frontend.
	GetFrontendURL() (string, bool)
}
