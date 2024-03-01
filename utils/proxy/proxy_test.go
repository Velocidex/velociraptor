package proxy

import (
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/vincent-petithory/dataurl"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type testCase struct {
	in    string
	check func(dest *url.URL)
}

func tableTest(t *testing.T,
	handler ProxyHandler, tests []testCase) {
	for _, test := range tests {
		src, err := url.Parse(test.in)
		assert.NoError(t, err)

		dest, err := handler.Handle(&http.Request{
			URL: src,
		})
		assert.NoError(t, err)
		test.check(dest)
	}
}

func destMatches(t *testing.T, regex string) func(dest *url.URL) {
	return func(dest *url.URL) {
		assert.Regexp(t, regex, dest.String())
	}
}

func destIsNil(t *testing.T) func(dest *url.URL) {
	return func(dest *url.URL) {
		assert.Nil(t, dest)
	}
}

func TestProxyNotConfigured(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Proxy is not configured - should give direct connection.
	proxy_config := &config_proto.ProxyConfig{}
	handler, err := configureProxy(logger, proxy_config)
	assert.NoError(t, err)

	tableTest(t, handler, []testCase{
		{in: "http://www.google.com", check: destIsNil(t)},
		{in: "https://www.google.com", check: destIsNil(t)},
	})

}

func TestProxyConfigured(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	proxy_config := &config_proto.ProxyConfig{
		// Only set one of the proxies.
		Http: "http://localhost:3128",
		ProxyUrlRegexp: map[string]string{
			// Connect directly to google
			"www.google.com":    "",
			"www.microsoft.com": "https://proxy.microsoft.com",
		},
	}
	handler, err := configureProxy(logger, proxy_config)
	assert.NoError(t, err)

	tableTest(t, handler, []testCase{
		// Random url should use localhost as proxy
		{in: "http://www.ms.com", check: destMatches(t, "localhost:3128")},
		{in: "https://www.ms.com", check: destMatches(t, "localhost:3128")},

		// Connect directly to google
		{in: "http://www.google.com", check: destIsNil(t)},
		{in: "https://www.google.com", check: destIsNil(t)},

		// For Microsoft go thropugh their proxy.
		{in: "http://www.microsoft.com", check: destMatches(t, "microsoft.com")},
		{in: "https://www.microsoft.com", check: destMatches(t, "microsoft.com")},
	})

}

func TestProxyConfiguredOverrideEnv(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Only override the http env variable.
	os.Setenv("HTTP_PROXY", "http://proxy.google.com")
	defer os.Setenv("HTTP_PROXY", "")

	proxy_config := &config_proto.ProxyConfig{
		// This will be override by the env
		Http: "http://localhost:3128",

		// This will be honored still
		Https: "https://localhost:3128",
	}
	handler, err := configureProxy(logger, proxy_config)
	assert.NoError(t, err)

	tableTest(t, handler, []testCase{
		// Random url should use localhost as proxy
		{in: "http://www.ms.com", check: destMatches(t, "proxy.google.com")},
		{in: "https://www.ms.com", check: destMatches(t, "localhost:3128")},
	})

	// Configure to ignore the environment
	proxy_config = &config_proto.ProxyConfig{
		// This will be override by the env
		Http: "http://localhost:3128",

		// This will be honored still
		Https: "https://localhost:3128",

		IgnoreEnvironment: true,
	}
	handler, err = configureProxy(logger, proxy_config)
	assert.NoError(t, err)

	tableTest(t, handler, []testCase{
		// Completely ignore the environment now
		{in: "http://www.ms.com", check: destMatches(t, "localhost:3128")},
		{in: "https://www.ms.com", check: destMatches(t, "localhost:3128")},
	})
}

func TestPACConfiguration(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	pac_data := `
           function FindProxyForURL(url, host) {
		     // If the hostname matches, send direct.
		     if (dnsDomainIs(host, "www.google.com")) {
		         return "DIRECT";
		     }
		     return "HTTP localhost:8080";
		   }`

	proxy_config := &config_proto.ProxyConfig{
		// PAC file can be specified in a number of URL formats
		// including data url.
		Pac: dataurl.New([]byte(pac_data), "text/plain").String(),
	}

	handler, err := configureProxy(logger, proxy_config)
	assert.NoError(t, err)

	tableTest(t, handler, []testCase{
		// Contact google directly.
		{in: "http://www.google.com", check: destIsNil(t)},

		// All other URLs go through the localhost proxy
		{in: "https://www.ms.com", check: destMatches(t, "localhost:8080")},
	})
}
