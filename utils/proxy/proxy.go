// Configure the proxy

package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/jackwakefield/gopac"
	"github.com/vincent-petithory/dataurl"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

var (
	noConfigError = errors.New("No configuration")
)

type urlMatcher struct {
	re     *regexp.Regexp
	target *url.URL
}

func (self urlMatcher) String() string {
	if self.target == nil {
		return fmt.Sprintf("%v->NO_PROXY ", self.re.String())
	}
	return fmt.Sprintf("%v->%v ",
		self.re.String(), self.target.String())
}

func (self urlMatcher) Match(url_str string) (*url.URL, bool) {
	if self.re.MatchString(url_str) {
		return self.target, true
	}
	return nil, false
}

type ProxyHandler interface {
	String() string
	Handle(req *http.Request) (*url.URL, error)
}

// This proxy handler is fully configured by the config file.
type GenericProxyHandler struct {
	http_url, https_url *url.URL
	url_matchers        []urlMatcher
}

func (self GenericProxyHandler) String() string {
	if self.http_url == nil &&
		self.https_url == nil &&
		self.url_matchers == nil {
		return "No proxy configured"
	}

	var result string
	if self.https_url != nil &&
		self.http_url != nil {
		result += fmt.Sprintf("HTTP_PROXY=%v HTTPS_PROXY=%v ",
			self.http_url, self.https_url)
	}

	if len(self.url_matchers) > 0 {
		for _, m := range self.url_matchers {
			result += m.String()
		}
	}
	return result
}

func (self GenericProxyHandler) Handle(req *http.Request) (*url.URL, error) {
	url_str := req.URL.String()
	for _, m := range self.url_matchers {
		url, ok := m.Match(url_str)
		if ok {
			return url, nil
		}
	}

	if req.URL.Scheme == "http" {
		return self.http_url, nil
	}

	if req.URL.Scheme == "https" {
		return self.https_url, nil
	}

	// Direct connection for all other protocols.
	return nil, nil
}

// Proxy handler that is configured from a PAC file.
type PacHandler struct {
	parser  *gopac.Parser
	pac_url string
}

func (self PacHandler) String() string {
	return fmt.Sprintf("PAC file at %v", self.pac_url)
}

func (self PacHandler) Handle(req *http.Request) (*url.URL, error) {
	url_str, err := self.parser.FindProxy(req.URL.String(), req.URL.Host)
	if err != nil {
		return nil, err
	}

	// The response follows a specific format
	// https://developer.mozilla.org/en-US/docs/web/http/proxy_servers_and_tunneling/proxy_auto-configuration_pac_file
	if url_str == "" {
		return nil, nil
	}

	options := strings.Split(url_str, ";")
	// We do not support failover so just try the first and hope for
	// the best
	parts := strings.Split(strings.TrimSpace(options[0]), " ")
	switch strings.ToUpper(parts[0]) {
	case "DIRECT":
		return nil, nil

	case "HTTP":
		if len(parts) > 1 {
			return &url.URL{
				Scheme: "http",
				Host:   parts[1],
			}, nil
		}

	case "HTTPS":
		if len(parts) > 1 {
			return &url.URL{
				Scheme: "https",
				Host:   parts[1],
			}, nil
		}

		// A PROXY verb means to use the scheme of the original
		// request.
	case "PROXY":
		if len(parts) > 1 {
			return &url.URL{
				Scheme: req.URL.Scheme,
				Host:   parts[1],
			}, nil
		}
	}

	return nil, fmt.Errorf("PacProxyHandler: Unsupported proxy redirect %v",
		url_str)
}

// Also support data URLs
type DataURLTransport struct{}

func (self DataURLTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "data" {
		return nil, errors.New("Only data protocol supproted")
	}

	dataURL, err := dataurl.DecodeString(req.URL.String())
	if err != nil {
		return nil, err
	}

	return &http.Response{
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		Header:     make(http.Header),
		Close:      true,
		Body:       io.NopCloser(bytes.NewReader(dataURL.Data)),
	}, nil
}

func configureProxy(
	logger *logging.LogContext,
	proxy_config *config_proto.ProxyConfig) (
	handler ProxyHandler, err error) {
	if proxy_config.Pac != "" {
		// Suppoprt PAC files on the local filesystem as well with file:/// URLs.
		t := &http.Transport{}
		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
		t.RegisterProtocol("data", DataURLTransport{})

		http_client := &http.Client{Transport: t}

		logger.Info("ProxyHandler: Fetching PAC file from <green>%v</>", proxy_config.Pac)
		resp, err := http_client.Get(proxy_config.Pac)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, constants.MAX_MEMORY))
		if err != nil {
			return nil, err
		}

		// Now try to parse the PAC file
		handler := &PacHandler{
			parser:  &gopac.Parser{},
			pac_url: proxy_config.Pac,
		}
		err = handler.parser.ParseBytes(body)
		if err != nil {
			return nil, fmt.Errorf("ProxyConfig parsing PAC file: %w", err)
		}
		return handler, nil
	}

	var url_matchers []urlMatcher

	for re_str, target_str := range proxy_config.ProxyUrlRegexp {
		re, err := regexp.Compile("(?i)" + re_str)
		if err != nil {
			return nil, fmt.Errorf("ProxyConfig parsing direct_url_regexp: %w", err)
		}

		var target *url.URL

		// An empty target means no proxy
		if target_str != "" {
			target, err = url.Parse(target_str)
			if err != nil {
				return nil, fmt.Errorf(
					"ProxyConfig parsing direct_url_regexp: %w", err)
			}
		}
		url_matchers = append(url_matchers, urlMatcher{
			re: re, target: target})
	}

	var http_url_str, https_url_str string

	if !proxy_config.IgnoreEnvironment {
		http_url_str = getEnvAny("HTTP_PROXY", "http_proxy")
		https_url_str = getEnvAny("HTTPS_PROXY", "https_proxy")
	}

	if http_url_str == "" && proxy_config.Http != "" {
		http_url_str = proxy_config.Http
	}

	if https_url_str == "" && proxy_config.Https != "" {
		https_url_str = proxy_config.Https
	}

	// If one of http and https is empty copy from the other one.
	if http_url_str == "" && https_url_str != "" {
		http_url_str = https_url_str
	}

	if https_url_str == "" && http_url_str != "" {
		https_url_str = http_url_str
	}

	generic_handler := &GenericProxyHandler{
		url_matchers: url_matchers,
	}

	if http_url_str != "" {
		generic_handler.http_url, err = url.Parse(http_url_str)
		if err != nil {
			return nil, fmt.Errorf("ProxyConfig parsing http_url %v: %w",
				http_url_str, err)
		}

		generic_handler.https_url, err = url.Parse(https_url_str)
		if err != nil {
			return nil, fmt.Errorf("ProxyConfig parsing https_url %v: %w",
				https_url_str, err)
		}
	}

	return generic_handler, nil
}

func ConfigureProxy(config_obj *config_proto.Config) error {
	// Try to configur proxy set in the Frontend section first, but if
	// not present use the Client section.
	_, err := ConfigureFrontendProxy(config_obj)
	if err == noConfigError {
		_, err = ConfigureClientProxy(config_obj)
	}
	return err
}

func ConfigureFrontendProxy(config_obj *config_proto.Config) (ProxyHandler, error) {
	if config_obj.Frontend == nil {
		return nil, noConfigError
	}

	proxy_config := &config_proto.ProxyConfig{}
	if config_obj.Frontend.ProxyConfig != nil {
		proxy_config = config_obj.Frontend.ProxyConfig
	} else if config_obj.Frontend.Proxy != "" {
		proxy_config.Http = config_obj.Frontend.Proxy
		proxy_config.Https = config_obj.Frontend.Proxy
	} else {
		return nil, noConfigError
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	handler, err := configureProxy(logger, proxy_config)
	if err != nil {
		return nil, err
	}

	logger.Info("Setting proxy to <green>%v</>", handler.String())

	networking.SetProxy(handler.Handle)
	http_comms.SetProxy(handler.Handle)

	return handler, nil
}

func ConfigureClientProxy(config_obj *config_proto.Config) (ProxyHandler, error) {
	if config_obj.Client == nil {
		return nil, nil
	}

	proxy_config := &config_proto.ProxyConfig{}
	if config_obj.Client.ProxyConfig != nil {
		proxy_config = config_obj.Client.ProxyConfig
	} else if config_obj.Client.Proxy != "" {
		proxy_config.Http = config_obj.Client.Proxy
		proxy_config.Https = config_obj.Client.Proxy
	} else {
		// The client config does not have proxy set, so return a
		// default proxy handler.
		return nil, nil
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	handler, err := configureProxy(logger, proxy_config)
	if err != nil {
		return nil, err
	}

	logger.Info("Setting client proxy to <green>%v</>", handler.String())

	networking.SetProxy(handler.Handle)
	http_comms.SetProxy(handler.Handle)
	return handler, nil
}

func getEnvAny(names ...string) string {
	for _, n := range names {
		if val := os.Getenv(n); val != "" {
			return val
		}
	}
	return ""
}
