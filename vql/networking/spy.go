package networking

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	network_dialer_fd *os.File
)

func MaybeSpyOnWSDialer(
	config_obj *config_proto.Config,
	dialer *websocket.Dialer) *websocket.Dialer {

	if config_obj.Client == nil ||
		config_obj.Client.InsecureNetworkTraceFile == "" {
		return dialer
	}

	fd := GetTraceFile(config_obj)
	if fd == nil {
		return dialer
	}

	return spyOnWSDialer(dialer, fd)
}

func GetTraceFile(config_obj *config_proto.Config) *os.File {
	mu.Lock()
	defer mu.Unlock()

	if network_dialer_fd == nil {
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)

		fd, err := os.OpenFile(config_obj.Client.InsecureNetworkTraceFile,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
		if err != nil {
			logger.Error("MaybeSpyOnWSDialer: Unable to open network trace file %v: %v\n",
				config_obj.Client.InsecureNetworkTraceFile, err)
		}

		network_dialer_fd = fd
		logger.Info("<red>Insecure Spying on network connections</> in %v",
			config_obj.Client.InsecureNetworkTraceFile)
	}
	return network_dialer_fd
}

func spyOnWSDialer(dialer *websocket.Dialer, fd *os.File) *websocket.Dialer {
	dial := &net.Dialer{}
	dialer.NetDialTLSContext = func(ctx context.Context,
		network, addr string) (net.Conn, error) {
		c, err := dial.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		// Turn off TLS security for debugging
		cfg := new(tls.Config)
		cfg.InsecureSkipVerify = true

		tlsConn := tls.Client(c, cfg)

		errc := make(chan error, 2)
		timer := time.AfterFunc(time.Second, func() {
			errc <- errors.New("TLS handshake timeout")
		})

		go func() {
			err := tlsConn.Handshake()
			timer.Stop()
			errc <- err
		}()

		if err := <-errc; err != nil {
			c.Close()
			return nil, err
		}

		_, _ = fd.Write([]byte(fmt.Sprintf("\n--- %v %v->%v\n\n",
			utils.GetTime().Now().Format(time.RFC3339),
			network, addr)))
		return WrapConnection(tlsConn, fd), nil
	}

	return dialer
}

func MaybeSpyOnTransport(
	config_obj *config_proto.Config,
	transport *http.Transport) *http.Transport {

	if config_obj.Client == nil ||
		config_obj.Client.InsecureNetworkTraceFile == "" {
		return transport
	}

	fd := GetTraceFile(config_obj)
	if fd == nil {
		return transport
	}

	return spyOnTransport(transport, fd)

}

func spyOnTransport(transport *http.Transport, fd *os.File) *http.Transport {
	// Make a copy of the transport as we will change its dialer.
	transport = transport.Clone()

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport.Dial = func(network, address string) (net.Conn, error) {
		c, err := dialer.Dial(network, address)
		if err != nil {
			return nil, err
		}

		_, _ = fd.Write([]byte(fmt.Sprintf("\n--- %v %v->%v\n\n",
			utils.GetTime().Now().Format(time.RFC3339),
			network, address)))
		return WrapConnection(c, fd), nil
	}

	transport.DialTLS = func(network, address string) (net.Conn, error) {
		plainConn, err := dialer.Dial(network, address)
		if err != nil {
			return nil, err
		}

		// Turn off TLS security for debugging
		cfg := new(tls.Config)
		cfg.InsecureSkipVerify = true

		tlsConn := tls.Client(plainConn, cfg)

		errc := make(chan error, 2)
		timer := time.AfterFunc(time.Second, func() {
			errc <- errors.New("TLS handshake timeout")
		})

		go func() {
			err := tlsConn.Handshake()
			timer.Stop()
			errc <- err
		}()

		if err := <-errc; err != nil {
			plainConn.Close()
			return nil, err
		}

		_, _ = fd.Write([]byte(fmt.Sprintf("\n--- %v %v->%v\n\n",
			utils.GetTime().Now().Format(time.RFC3339),
			network, address)))
		return WrapConnection(tlsConn, fd), nil
	}

	return transport
}

func WrapConnection(c net.Conn, output *os.File) net.Conn {
	return &spyConnection{
		Conn: c,
		fd:   output,
	}
}

type spyConnection struct {
	net.Conn
	fd *os.File
}

func (self *spyConnection) Read(b []byte) (int, error) {
	n, err := self.Conn.Read(b)
	_, _ = self.fd.Write([]byte(fmt.Sprintf("\n--- %v %s->%s %v bytes\n\n%v",
		utils.GetTime().Now().Format(time.RFC3339),
		self.RemoteAddr(), self.LocalAddr(),
		n, string(b[:n]))))
	return n, err
}

func (self *spyConnection) Write(b []byte) (int, error) {
	n, err := self.Conn.Write(b)
	_, _ = self.fd.Write([]byte(fmt.Sprintf("\n--- %v  %s->%s %v bytes\n\n%v",
		utils.GetTime().Now().Format(time.RFC3339),
		self.LocalAddr(), self.RemoteAddr(),
		n, string(b[:n]))))
	return n, err
}
