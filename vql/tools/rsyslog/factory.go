package rsyslog

import (
	"context"
	"io"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils/syslog"
)

type syslogWriter struct {
	logger *Logger
}

func (self syslogWriter) Write(in []byte) (int, error) {
	return len(in), self.logger.Write(string(in))
}

// Make a temporary logger from a pool
func NewLogger(ctx context.Context,
	config_obj *config_proto.ClientConfig,
	network, raddr string,
	root_certs string,
	connectTimeout time.Duration) (io.Writer, error) {

	pool := NewConnectionPool(ctx)
	logger, err := pool.Dial(config_obj, network, raddr, root_certs, connectTimeout)
	if err != nil {
		return nil, err
	}

	return &syslogWriter{logger}, nil
}

func init() {
	syslog.Factory = NewLogger
}
