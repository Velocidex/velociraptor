package networking

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"www.velocidex.com/golang/velociraptor/config/proto"
)

func GetHttpTransport(config_obj *proto.ClientConfig, extra_roots string) (*http.Transport, error) {
	timeout := config_obj.ConnectionTimeout
	if timeout == 0 {
		timeout = 300 // 5 Min default
	}

	tlsConfig, err := GetTlsConfig(config_obj, extra_roots)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		d := net.Dialer{
			Timeout:   time.Duration(timeout) * time.Second,
			KeepAlive: time.Duration(timeout) * time.Second,
			DualStack: true,
		}

		// try default dial with DNS resolution (if necessary)
		conn, err := d.DialContext(ctx, network, addr)
		if err == nil {
			return conn, nil
		}

		// if the attempt failed, check whether there is a fallback address in the config
		fallback, pres := config_obj.GetFallbackAddresses()[addr]
		if !pres {
			return nil, err
		}
		return d.DialContext(ctx, network, fallback)
	}
	transport.Proxy = proxyHandler
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = time.Duration(timeout) * time.Second
	transport.TLSHandshakeTimeout = time.Duration(timeout) * time.Second
	transport.ExpectContinueTimeout = time.Duration(timeout) * time.Second
	transport.ResponseHeaderTimeout = time.Duration(timeout) * time.Second

	// disable HTTP/2, apparently it's bugged in recent Go versions
	transport.TLSNextProto = make(map[string]func(
		authority string, c *tls.Conn) http.RoundTripper)

	return transport, nil
}
