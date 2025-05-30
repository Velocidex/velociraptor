package networking

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

// Copies the src_arg into a new arg with secrets expanded into it.
func (self *HTTPClientCache) mergeSecretToRequest(
	ctx context.Context, scope vfilter.Scope,
	src_arg *HttpPluginRequest,
	url_obj *url.URL) (*HttpPluginRequest, error) {

	if url_obj.Scheme != "secret" {
		return src_arg, nil
	}

	secret_name := url_obj.Host

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil, errors.New("Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return nil, err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	s, err := secrets_service.GetSecret(ctx, principal,
		constants.HTTP_SECRETS, secret_name)
	if err != nil {
		return nil, err
	}

	// Shallow copy - we replace the args with the secret
	arg := *src_arg

	if arg.Params == nil {
		arg.Params = ordereddict.NewDict()
	}

	if arg.Headers == nil {
		arg.Headers = ordereddict.NewDict()
	}

	if arg.CookieJar == nil {
		arg.CookieJar = ordereddict.NewDict()
	}

	// Our copy has only one url and consists of the expanded set.
	arg.Url = []string{url_obj.String()}

	// The real url is hidden so it does not get logged.
	real_url_str := ""
	s.GetString("url", &real_url_str)

	// Validate the URL
	arg.real_url, err = url.Parse(real_url_str)
	if err != nil {
		return nil, fmt.Errorf(
			"http_client: HTTP Secret has invalid URL: %v: %w",
			secret_name, err)
	}

	url_regex := ""
	s.GetString("url_regex", &url_regex)
	if url_regex != "" {
		re, err := regexp.Compile(url_regex)
		if err != nil {
			return nil, fmt.Errorf(
				"http_client: HTTP Secret has invalid URL regex: %v: %w",
				url_regex, err)
		}

		if !re.MatchString(real_url_str) {
			return nil, fmt.Errorf(
				"http_client: HTTP Secret URL regex %v forbids connection to %v",
				url_regex, url_obj.String())
		}
	}

	// Currently secrets only support a single URL
	s.GetString("method", &arg.Method)
	s.GetString("user_agent", &arg.UserAgent)
	s.GetString("root_ca", &arg.RootCerts)
	s.GetBool("skip_verify", &arg.SkipVerify)
	s.GetDict("extra_params", arg.Params)
	s.GetDict("extra_headers", arg.Headers)
	s.GetDict("cookies", arg.CookieJar)

	return &arg, nil
}
