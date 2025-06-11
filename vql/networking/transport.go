package networking

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"www.velocidex.com/golang/velociraptor/config/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetHttpTransport(config_obj *proto.ClientConfig, extra_roots string) (*http.Transport, error) {
	if config_obj == nil {
		config_obj = &config_proto.ClientConfig{}
	}

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
	transport.DialContext = func(ctx context.Context, network, addr string) (
		c net.Conn, ret_err error) {

		d := net.Dialer{
			Timeout:   time.Duration(timeout) * time.Second,
			KeepAlive: time.Duration(timeout) * time.Second,
			DualStack: true,
		}

		ips, err := getLookupAddresses(ctx, config_obj, addr)
		if err == nil {
			if len(ips) > 0 {
				for _, ip := range ips {
					// try default dial with DNS resolution (if necessary)
					conn, err := d.DialContext(ctx, network, ip)
					if err == nil {
						return conn, nil
					}
					ret_err = err
				}
			} else {
				ret_err = errors.New("No IPs resolvable")
			}

		} else {
			ret_err = err
		}

		// As a fallback get any addresses in the config file
		fallback_addresses := config_obj.FallbackAddresses
		if fallback_addresses != nil {
			fallback, pres := fallback_addresses[addr]
			if pres {
				conn, err := d.DialContext(ctx, network, fallback)
				if err == nil {
					return conn, nil
				}
				ret_err = err
			}
		}

		return nil, ret_err
	}

	transport.Proxy = proxyHandler
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = time.Duration(timeout) * time.Second
	transport.TLSHandshakeTimeout = time.Duration(timeout) * time.Second
	transport.ExpectContinueTimeout = time.Duration(timeout) * time.Second
	transport.ResponseHeaderTimeout = time.Duration(timeout) * time.Second

	return transport, nil
}
