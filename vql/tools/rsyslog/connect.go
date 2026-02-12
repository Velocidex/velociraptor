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

var (
	OverflowError = utils.Wrap(utils.IOError, "Syslog message overflow")
)

type Logger struct {
	ctx            context.Context
	netConn        net.Conn
	config_obj     *config_proto.ClientConfig
	network, raddr string
	root_certs     string
	connectTimeout time.Duration

	messageQueue chan string
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

// Drain messages from the queue and send them. The queue provides
// some flexibility to handle spikes in messages for a while.
func (self *Logger) Start() {
	for {
		select {
		case <-self.ctx.Done():
			self.Close()
			return

		case msg, ok := <-self.messageQueue:
			if !ok {
				return
			}
			err := self._Write(msg)
			if err != nil {
				fmt.Printf("Writing to %v: %v \n", self.raddr, err)
			}
		}
	}
}

func (self *Logger) Close() {
	if self.netConn != nil {
		self.netConn.Close()
		files.Remove(self.raddr)
	}
}

func (self *Logger) Write(message string) (err error) {
	select {
	case <-self.ctx.Done():
		return utils.CancelledError

	case self.messageQueue <- message:
		return nil

	default:
		return OverflowError
	}
}

func (self *Logger) _Write(message string) (err error) {
	// Retry 3 times
	for i := 0; i < 3; i++ {
		if self.netConn == nil {
			err = self.Connect()
			if err != nil {
				utils.SleepWithCtx(self.ctx, time.Second)
				continue
			}
		}

		deadline := time.Now().Add(self.connectTimeout)
		switch self.network {
		case "tcp", "tls":
			_ = self.netConn.SetWriteDeadline(deadline)
			message = fmt.Sprintf("%d %s\n", len(message)+1, message)
			_, err = io.WriteString(self.netConn, message)

		case "udp":
			_ = self.netConn.SetWriteDeadline(deadline)
			if len(message) > 1024 {
				message = message[:1024]
			}

			_, err = io.WriteString(self.netConn, message)

		default:
			return fmt.Errorf("Network protocol %s not supported: %T",
				self.network, self.netConn)
		}
		if err == nil {
			return nil
		}

		// We had an error -- we need to close the connection and try again
		self.netConn.Close()

		utils.SleepWithCtx(self.ctx, time.Second)
		err = self.Connect()
		if err != nil {
			continue
		}

		select {
		case <-self.ctx.Done():
			return utils.CancelledError
		default:
		}
	}

	return err
}
