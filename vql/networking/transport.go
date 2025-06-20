package networking

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/config/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	TransportCache = &TransportCacheType{
		cache: make(map[string]*http.Transport),
	}
)

type TransportCacheType struct {
	mu    sync.Mutex
	cache map[string]*http.Transport
}

func (self *TransportCacheType) Reset() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.cache = make(map[string]*http.Transport)
}

func (self *TransportCacheType) Get(extra_roots string) (*http.Transport, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.cache[extra_roots]
	return res, pres
}

func (self *TransportCacheType) Set(extra_roots string, t *http.Transport) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.cache[extra_roots] = t
}

// Try to cache the HTTP Transport to allow it to use connections better.
func GetHttpTransport(config_obj *proto.ClientConfig, extra_roots string) (*http.Transport, error) {

	transport, pres := TransportCache.Get(extra_roots)
	if pres {
		return transport, nil
	}

	transport, err := GetNewHttpTransport(config_obj, extra_roots)

	if err == nil {
		TransportCache.Set(extra_roots, transport)
	}

	return transport, err
}

// Create a new transport without caching it.
func GetNewHttpTransport(config_obj *proto.ClientConfig, extra_roots string) (
	*http.Transport, error) {
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
						return gConnectionTracker.NewTrackedConnection(conn, addr), nil
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
					return gConnectionTracker.NewTrackedConnection(conn, addr), nil
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
