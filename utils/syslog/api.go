// Provides an implementation of a syslog logger

package syslog

import (
	"context"
	"io"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	Factory func(ctx context.Context,
		config_obj *config_proto.ClientConfig,
		network, raddr string,
		root_certs string,
		connectTimeout time.Duration) (io.Writer, error) = nil
)

func NewHook(ctx context.Context,
	config_obj *config_proto.ClientConfig,
	network, raddr string,
	root_certs string,
	connectTimeout time.Duration) (logrus.Hook, error) {
	if Factory == nil {
		return nil, utils.Wrap(utils.NotImplementedError, "Syslog factory not initialized")
	}

	writer_fd, err := Factory(ctx, config_obj, network, raddr, root_certs, connectTimeout)
	if err != nil {
		return nil, err
	}

	return &writer.Hook{
		Writer: writer_fd,
		LogLevels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
			logrus.InfoLevel,
		},
	}, nil
}
