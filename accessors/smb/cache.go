package smb

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	errors "github.com/go-errors/errors"
	"github.com/hirochachacha/go-smb2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/utils/dict"
)

var (
	smbAccessorCurrentOpened = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_smb_current_open_files",
		Help: "Number of currently opened files with the smb accessor.",
	})

	smbAccessorTotalRemoteMounts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accessor_smb_total_mounts",
		Help: "Total Number of times the SMB accessor mounted a remote share",
	})

	smbAccessorCurrentRemoteMounts = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_smb_current_mounts",
		Help: "Total Number of times the SMB accessor mounted a remote share",
	})
)

type SMBConnectionContext struct {
	mu      sync.Mutex
	err     error
	server  string
	conn    net.Conn
	session *smb2.Session
	mount   map[string]*smb2.Share
}

func NewSMBConnectionContext(
	ctx context.Context, scope vfilter.Scope,
	server_name string) (*SMBConnectionContext, error) {

	creds, err := getCreadentials(ctx, scope, server_name)
	if err != nil {
		return nil, err
	}

	if !strings.Contains(server_name, ":") {
		server_name += ":445"
	}

	conn, err := net.Dial("tcp", server_name)
	if err != nil {
		return nil, err
	}

	d := &smb2.Dialer{
		Initiator: creds,
	}

	session, err := d.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &SMBConnectionContext{
		server:  server_name,
		conn:    conn,
		session: session,
		mount:   make(map[string]*smb2.Share),
	}, nil
}

func (self *SMBConnectionContext) Session() *smb2.Session {
	return self.session
}

func (self *SMBConnectionContext) Mount(name string) (*smb2.Share, error) {
	share, pres := self.mount[name]
	if pres {
		return share, nil
	}

	fs, err := self.session.Mount(name)
	if err != nil {
		return nil, err
	}
	self.mount[name] = fs

	smbAccessorTotalRemoteMounts.Inc()
	smbAccessorCurrentRemoteMounts.Inc()
	return fs, nil
}

func (self *SMBConnectionContext) Close() {
	if self.session != nil {
		_ = self.session.Logoff()
	}
	if self.conn != nil {
		self.conn.Close()
	}
	smbAccessorCurrentRemoteMounts.Sub(float64(len(self.mount)))
}

type SMBMountCache struct {
	mu    sync.Mutex
	ctx   context.Context
	scope vfilter.Scope
	lru   *ttlcache.Cache // map[server]*SMBConnectionContext
}

func (self *SMBMountCache) GetHandle(server_name string) (
	*SMBConnectionContext, func(), error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	cached_any, err := self.lru.Get(server_name)
	if err == nil {
		cached, ok := cached_any.(*SMBConnectionContext)
		if ok {
			cached.mu.Lock()
			err := cached.err
			if err != nil {
				cached.mu.Unlock()
				return nil, nil, err
			}
			return cached, cached.mu.Unlock, nil
		}
	}

	// Create a new context
	cached, err := NewSMBConnectionContext(self.ctx, self.scope, server_name)
	if err != nil {
		// Cache the failure - this usually means wrong creds.
		cached = &SMBConnectionContext{
			err: err,
		}
	}

	// Set to refresh the TTL
	_ = self.lru.Set(server_name, cached)
	cached.mu.Lock()
	return cached, cached.mu.Unlock, err
}

func NewSMBMountCache(scope vfilter.Scope) *SMBMountCache {
	// Tie our lifetime to the root scope.
	ctx, cancel := context.WithCancel(context.Background())
	result := &SMBMountCache{
		ctx:   ctx,
		scope: scope,
		lru:   ttlcache.NewCache(),
	}
	_ = result.lru.SetTTL(time.Hour)
	result.lru.SetExpirationCallback(
		func(key string, value interface{}) error {
			ctx, ok := value.(*SMBConnectionContext)
			if ok {
				// Do not block the lru while closing.
				go ctx.Close()
			}
			return nil
		})

	_ = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
		result.lru.Flush()
		result.lru.Close()
		cancel()
	})
	return result
}

func getCreadentials(
	ctx context.Context, scope vfilter.Scope, hostname string) (
	*smb2.NTLMInitiator, error) {

	credentials, pres := scope.Resolve(constants.SMB_CREDENTIALS)
	if !pres {
		return nil, errors.New("No credentials provided for smb connections")
	}

	args := dict.RowToDict(ctx, scope, credentials)

	var creds string
	arg := &SMBAccessorArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		// Try to support the old style args for backwards compatibility.
		creds, pres = args.GetString(hostname)
	} else {
		creds, pres = arg.Hosts.GetString(hostname)
	}

	if !pres {
		return nil, fmt.Errorf("No credentials found for %v", hostname)
	}
	parts := strings.SplitN(creds, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("Invalid credentials provided for %v", hostname)
	}

	return &smb2.NTLMInitiator{
		User:     parts[0],
		Password: parts[1],
	}, nil
}
