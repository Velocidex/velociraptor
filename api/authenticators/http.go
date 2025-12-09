package authenticators

import (
	"bytes"
	"context"
	"io"
	"net/http"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type transformerTransport struct {
	transport    http.RoundTripper
	transformers []Transformer
}

func (self *transformerTransport) RoundTrip(
	req *http.Request) (*http.Response, error) {

	rt := self.transport.RoundTrip
	if rt == nil {
		rt = http.DefaultTransport.RoundTrip
	}

	for _, t := range self.transformers {
		rt = t(rt)
	}

	return rt(req)
}

// The OIDC libraries force us to embed the http client inside the
// context but this is error prone because we can accidentally feed
// them a regular context (without a custom http client). This
// transparently uses the default http client which breaks in cases we
// need proxies etc.
// To fix this we require a new type for a HTTP adorned context.
type HTTPClientContext struct {
	context.Context
	HTTPClient *http.Client
}

// Update the HTTP client in the context honoring proxy and TLS
// settings in the config file. This is needed to pass to oidc
// functions that will make HTTP calls.
func ClientContext(
	ctx context.Context,
	config_obj *config_proto.Config,
	transformers []Transformer) (*HTTPClientContext, error) {
	transport, err := networking.GetHttpTransport(config_obj.Client, "")
	if err != nil {
		return nil, err
	}

	// Allow the context to be spied on if needed.
	transport = networking.MaybeSpyOnTransport(config_obj, transport)

	client := &http.Client{
		Transport: &transformerTransport{
			transport:    transport,
			transformers: transformers,
		},
	}

	return &HTTPClientContext{
		Context:    oidc.ClientContext(ctx, client),
		HTTPClient: client,
	}, nil
}

func DefaultTransforms(
	config_obj *config_proto.Config,
	authenticator *config_proto.Authenticator) []Transformer {
	if authenticator.OidcDebug {
		return []Transformer{traceNetwork(config_obj)}
	}
	return nil
}

func traceNetwork(config_obj *config_proto.Config) func(rt RoundTripFunc) RoundTripFunc {
	return func(rt RoundTripFunc) RoundTripFunc {
		return func(req *http.Request) (*http.Response, error) {
			res, err := rt(req)
			if err != nil {
				return nil, err
			}
			defer res.Body.Close()

			ct, _ := res.Header["Content-Type"]

			bs, err := io.ReadAll(res.Body)
			if err != nil {
				return nil, err
			}

			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Debug("oidc: Calling URL: %v, Response: %v (CT %v)",
				req.URL, string(bs), ct)

			res.Body = io.NopCloser(bytes.NewReader(bs))
			return res, nil
		}
	}
}
