package networking

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func testHTTPConnection(
	config_obj *config_proto.ClientConfig, url string) (
	HTTPClient, []byte, error) {

	TransportCache.Reset()

	url_obj, err := parseURL(url)
	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	client, _, err := GetHttpClient(ctx, config_obj, scope, &HttpPluginRequest{
		Url:    []string{url},
		Method: "GET",
	}, url_obj)
	if err != nil {
		return client, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx, "GET", url, strings.NewReader(""))
	if err != nil {
		return client, nil, err
	}

	http_resp, err := client.Do(req)
	if err != nil {
		return client, nil, err
	}
	defer http_resp.Body.Close()

	res, err := ioutil.ReadAll(http_resp.Body)
	return client, res, err
}

func TestTLSVerification(t *testing.T) {
	ts := vtesting.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// Default client config is PKI verification - we dont like this
	// certificate.
	config_obj := &config_proto.ClientConfig{}
	_, _, err := testHTTPConnection(config_obj, ts.URL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown authority")

	config_obj = &config_proto.ClientConfig{
		Crypto: &config_proto.CryptoConfig{
			CertificateVerificationMode: "PKI",
			// We still ignore the thumbprint because we are in PKI
			// mode.
			CertificateThumbprints: []string{
				"AB:60:19:14:43:6E:58:BA:BB:17:B9:16:61:55:CA:F9:7B:D7:E5:F8:DE:B9:B6:59:BC:DB:66:C5:8B:49:F3:23",
			},
		},
	}
	_, _, err = testHTTPConnection(config_obj, ts.URL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown authority")

	// Allowing the certificate fingerprint will trust it.
	config_obj = &config_proto.ClientConfig{
		Crypto: &config_proto.CryptoConfig{
			CertificateVerificationMode: "PKI_OR_THUMBPRINT",
			CertificateThumbprints: []string{
				"AB:60:19:14:43:6E:58:BA:BB:17:B9:16:61:55:CA:F9:7B:D7:E5:F8:DE:B9:B6:59:BC:DB:66:C5:8B:49:F3:23",
			},
		},
	}
	_, data, err := testHTTPConnection(config_obj, ts.URL)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "Hello, client")

	// In THUMBPRINT_ONLY mode we only trust pinned certs.
	config_obj = &config_proto.ClientConfig{
		Crypto: &config_proto.CryptoConfig{
			CertificateVerificationMode: "THUMBPRINT_ONLY",
			CertificateThumbprints: []string{
				"AB601914436E58BABB17B9166155CAF97BD7E5F8DEB9B659BCDB66C58B49F323",
			},
		},
	}
	_, data, err = testHTTPConnection(config_obj, ts.URL)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "Hello, client")

	// In THUMBPRINT_ONLY mode we REJECT connections to proper TLS
	// servers
	config_obj = &config_proto.ClientConfig{
		Crypto: &config_proto.CryptoConfig{
			CertificateVerificationMode: "THUMBPRINT_ONLY",
			CertificateThumbprints: []string{
				"AB601914436E58BABB17B9166155CAF97BD7E5F8DEB9B659BCDB66C58B49F323",
			},
		},
	}
	_, data, err = testHTTPConnection(config_obj, "https://www.google.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Server certificate had no known thumbprint")

	// Test fallback addresses - when DNS fails, we substitute
	// connection to the IP address and pin the certificate.
	config_obj = &config_proto.ClientConfig{
		Crypto: &config_proto.CryptoConfig{
			CertificateVerificationMode: "THUMBPRINT_ONLY",
			CertificateThumbprints: []string{
				"AB601914436E58BABB17B9166155CAF97BD7E5F8DEB9B659BCDB66C58B49F323",
			},
		},
		FallbackAddresses: map[string]string{
			"nosuch-site.example.com:443": strings.TrimPrefix(ts.URL, "https://"),
		},
	}
	_, data, err = testHTTPConnection(config_obj, "https://nosuch-site.example.com")
	assert.NoError(t, err)
	assert.Contains(t, string(data), "Hello, client")
}
