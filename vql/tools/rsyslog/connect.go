package rsyslog

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/files"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type Logger struct {
	netConn        net.Conn
	config_obj     *config_proto.ClientConfig
	network, raddr string
	root_certs     string
	connectTimeout time.Duration
}

func (self *Logger) Connect() (err error) {
	switch self.network {
	case "tls":
		tls_config, err := networking.GetTlsConfig(self.config_obj, self.root_certs)
		if err != nil {
			return err
		}

		dialer := &net.Dialer{
			Timeout:   self.connectTimeout,
			KeepAlive: time.Second * 60,
		}
		self.netConn, err = tls.DialWithDialer(dialer, "tcp", self.raddr, tls_config)

	case "udp", "tcp":
		self.netConn, err = net.DialTimeout(self.network, self.raddr, self.connectTimeout)

	default:
		return fmt.Errorf("Network protocol %s not supported", self.network)
	}

	if err != nil {
		return err
	}

	// Add reference to this connection which should be closed.
	files.Add(self.raddr)

	return nil
}

func (self *Logger) Close() {
	self.netConn.Close()
	files.Remove(self.raddr)
}

func (self *Logger) Write(ctx context.Context, message string) (err error) {
	// Retry 3 times
	for i := 0; i < 3; i++ {
		deadline := time.Now().Add(self.connectTimeout)
		switch self.netConn.(type) {
		case *net.TCPConn, *tls.Conn:
			self.netConn.SetWriteDeadline(deadline)
			_, err = io.WriteString(self.netConn, message+"\n")

		case *net.UDPConn:
			self.netConn.SetWriteDeadline(deadline)
			if len(message) > 1024 {
				message = message[:1024]
			}

			_, err = io.WriteString(self.netConn, message)
		default:

			return fmt.Errorf("Network protocol %s not supported", self.network)
		}
		if err == nil {
			return
		}

		// We had an error -- we need to close the connection and try again
		self.netConn.Close()

		utils.SleepWithCtx(ctx, time.Second)
		err = self.Connect()
		if err != nil {
			continue
		}

		select {
		case <-ctx.Done():
			return utils.CancelledError
		default:
		}
	}

	return err
}

func (self *connectionPool) Dial(config_obj *config_proto.ClientConfig,
	network, raddr string,
	root_certs string,
	connectTimeout time.Duration) (*Logger, error) {

	// Check if the connection is already in the pool
	existing, err := self.lru.Get(raddr)
	if err == nil {
		res, ok := existing.(*Logger)
		if ok {
			return res, nil
		}
	}

	res := &Logger{
		config_obj:     config_obj,
		network:        network,
		raddr:          raddr,
		root_certs:     root_certs,
		connectTimeout: connectTimeout,
	}

	self.lru.Set(raddr, res)

	return res, res.Connect()
}
