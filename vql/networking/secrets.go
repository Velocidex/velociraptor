package networking

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

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

	secret_name := src_arg.Secret
	if url_obj.Scheme == "secret" {
		secret_name = url_obj.Host
	}

	// No secrets involved
	if secret_name == "" {
		return src_arg, nil
	}

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

	// Currently secrets only support a single URL
	s.UpdateString("method", &arg.Method)

	// Normalize the method
	arg.Method = strings.ToUpper(arg.Method)
	if arg.Method == "" {
		arg.Method = "GET"
	}

	s.UpdateString("user_agent", &arg.UserAgent)
	s.UpdateString("root_ca", &arg.RootCerts)
	s.UpdateBool("skip_verify", &arg.SkipVerify)
	err = s.UpdateDict("extra_params", arg.Params)
	if err != nil {
		return nil, err
	}
	err = s.UpdateDict("extra_headers", arg.Headers)
	if err != nil {
		return nil, err
	}

	err = s.UpdateDict("cookies", arg.CookieJar)
	if err != nil {
		return nil, err
	}

	// Our copy has only one url and consists of the expanded set.
	arg.Url = []string{url_obj.String()}

	// The real url is hidden so it does not get logged.
	real_url_str := s.GetString("url")

	// The secret does not specify the url. This can happen if the
	// secret has a url regex instead.
	if real_url_str == "" {
		arg.real_url, err = url.Parse(arg.Url[0])
		if err != nil {
			return nil, fmt.Errorf(
				"http_client: HTTP Secret has invalid URL: %v: %w",
				secret_name, err)
		}

		// For get requests we need to merge the parameters in the
		// user URL.
		if arg.Method == "GET" {
			for k, v := range url_obj.Query() {
				// Do not allow the user to override the parameters in
				// the secret.
				_, pres := arg.Params.Get(k)
				if !pres {
					arg.Params.Set(k, v)
				}
			}
		}

	} else {
		arg.real_url, err = url.Parse(real_url_str)
	}
	if err != nil {
		return nil, fmt.Errorf(
			"http_client: HTTP Secret has invalid URL: %v: %w",
			secret_name, err)
	}

	return &arg, nil
}

func (self *_HttpPlugin) maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope,
	arg *HttpPluginRequest) {

	// Not running on the server, secrets dont work.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return
	}

	// The Security part of the config is normally only on the server.
	if config_obj.Security == nil ||

		// The default is to allow arbitrary URL access.
		!config_obj.Security.VqlMustUseSecrets {
		return
	}

	// If an explicit secret is defined let it filter the URLs.
	if arg.Secret != "" {
		return
	}

	// If we have to use secrets we must filter all the urls which are
	// not secrets.
	filtered_urls := make([]string, 0, len(arg.Url))
	for _, url := range arg.Url {
		url_obj, err := parseURL(url)
		if err != nil {
			scope.Log("http_client: parsing %s: %v", url, err)
			continue
		}

		if url_obj.Scheme != "secret" {
			scope.Log("http_client: must use secrets is enforced, dropping url %s", url)
			continue
		}

		filtered_urls = append(filtered_urls, url)
	}

	arg.Url = filtered_urls
}

func (self *_HttpPlugin) filterURLsWithSecret(
	ctx context.Context,
	scope vfilter.Scope,
	urls []string, secret_name string) ([]string, error) {

	// If there is no secret service we might be running on the
	// client, so there is no secret support.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return urls, nil
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

	url_regex := s.GetString("url_regex")
	if url_regex != "" {
		re, err := regexp.Compile(url_regex)
		if err != nil {
			return nil, fmt.Errorf(
				"http_client: HTTP Secret has invalid URL regex: %v: %w",
				url_regex, err)
		}

		var res []string
		for _, url := range urls {
			if !re.MatchString(url) {
				scope.Log("http_client: HTTP Secret URL regex %v forbids connection to %v",
					url_regex, url)
			} else {
				res = append(res, url)
			}
		}
		if len(res) == 0 {
			return nil, fmt.Errorf("http_client: HTTP Secret excludes all URLs")
		}
		return res, nil
	}

	return []string{"secret://" + secret_name}, nil
}
