package rsyslog

import (
	"context"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	SYSLOG_POOL_TAG = "$SYSLOG"
)

type connectionPool struct {
	ctx     context.Context
	loggers map[string]*Logger
}

// Get a new Logger for the specified address
func (self *connectionPool) Dial(
	config_obj *config_proto.ClientConfig,
	network, raddr string,
	root_certs string,
	connectTimeout time.Duration) (*Logger, error) {

	// Check if the connection is already in the pool
	existing, ok := self.loggers[raddr]
	if ok {
		return existing, nil
	}

	// No logger found, make a new one.
	res := &Logger{
		ctx:            self.ctx,
		config_obj:     config_obj,
		network:        network,
		raddr:          raddr,
		root_certs:     root_certs,
		connectTimeout: connectTimeout,
		messageQueue:   make(chan string, 1000),
	}

	err := res.Connect()
	if err != nil {
		return nil, err
	}

	go res.Start()

	self.loggers[raddr] = res

	return res, nil
}

// A pool manages access to several syslog loggers. The pool's life
// span is managed using the ctx.
func NewConnectionPool(ctx context.Context) *connectionPool {
	return &connectionPool{
		ctx:     ctx,
		loggers: make(map[string]*Logger),
	}
}

// Get a pool stored in the scope for the lifetime of the query.
func GetPool(ctx context.Context, scope vfilter.Scope) *connectionPool {
	pool_any := vql_subsystem.CacheGet(scope, SYSLOG_POOL_TAG)
	if pool_any != nil {
		pool, ok := pool_any.(*connectionPool)
		if ok {
			return pool
		}
	}

	pool := NewConnectionPool(ctx)
	vql_subsystem.CacheSet(scope, SYSLOG_POOL_TAG, pool)

	return pool
}
