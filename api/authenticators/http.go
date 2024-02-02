package authenticators

import (
	"net/http"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	context "golang.org/x/net/context"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

// Update the HTTP client in the context honoring proxy and TLS
// settings in the config file. This is needed to pass to
// oidc.NewProvider
func ClientContext(ctx context.Context,
	config_obj *config_proto.Config) (context.Context, error) {
	transport, err := networking.GetHttpTransport(config_obj.Client, "")
	if err != nil {
		return nil, err
	}

	client := &http.Client{Transport: transport}
	return oidc.ClientContext(ctx, client), nil
}
