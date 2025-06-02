package networking

import (
	"context"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	vfilter "www.velocidex.com/golang/vfilter"
)

type DNSResolver interface {
	LookupHost(ctx context.Context, host string) (addrs []string, err error)
}

var (
	DefaultResolver DNSResolver = net.DefaultResolver
)

// Expands the address into a list of lookup addresses. If the
// dnscache resolver is installed, we can replace the names with IP
// addresses directly. Also adds any fallback addresses if configured.
func getLookupAddresses(
	ctx context.Context, config_obj *config_proto.ClientConfig,
	addr string) (res []string, err error) {

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ips, err := DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}

	for _, ip := range ips {
		res = append(res, net.JoinHostPort(ip, port))
	}

	return res, nil
}

type cacheEntry struct {
	host         string
	rrs          []string
	last_refresh time.Time
	last_used    time.Time
	used         bool
}

type CachingResolver struct {
	mu    sync.Mutex
	cache map[string]*cacheEntry

	ctx     context.Context
	Timeout time.Duration

	LastRefresh time.Time
}

func (self *CachingResolver) LookupHost(
	ctx context.Context, host string) (addrs []string, err error) {
	self.mu.Lock()
	entry, pres := self.cache[host]
	if pres {
		entry.used = true
		rrs := entry.rrs
		defer self.mu.Unlock()
		return rrs, nil
	}
	self.mu.Unlock()

	// Perform the lookup and insert into the cache without blocking
	// the cache.
	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err == nil {
		self.mu.Lock()

		now := utils.GetTime().Now()
		self.cache[host] = &cacheEntry{
			rrs:          ips,
			last_used:    now,
			last_refresh: now,
			used:         true,
		}
		self.mu.Unlock()
	}

	return ips, err
}

func (self *CachingResolver) RefreshOnce() {
	self.mu.Lock()
	hosts := []string{}
	for host, entry := range self.cache {
		// Ignore entries that were not used since the last refresh.
		if entry.used {
			hosts = append(hosts, host)
			entry.used = false

		} else {
			delete(self.cache, host)
		}
	}
	self.LastRefresh = utils.GetTime().Now()
	self.mu.Unlock()

	for _, host := range hosts {
		// Lookup the host again to refresh the cache.
		ctx, cancel := context.WithTimeout(self.ctx, self.Timeout)
		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		if err == nil {
			self.mu.Lock()
			// A lookup might have happened during this run, just
			// reuse its entry so we can preserve the used status
			existing_entry, pres := self.cache[host]
			if !pres {
				existing_entry = &cacheEntry{}
			}

			existing_entry.rrs = ips
			existing_entry.last_refresh = utils.GetTime().Now()

			self.cache[host] = existing_entry
			self.mu.Unlock()
		}
		cancel()
	}
}

func (self *CachingResolver) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	var hosts []*cacheEntry
	for host, entry := range self.cache {
		copy := *entry
		copy.host = host
		hosts = append(hosts, &copy)
	}
	last_refresh := self.LastRefresh
	self.mu.Unlock()

	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].host < hosts[j].host
	})

	now := utils.GetTime().Now()
	for _, entry := range hosts {
		output_chan <- ordereddict.NewDict().
			Set("Host", entry.host).
			Set("IPs", entry.rrs).
			Set("LastUsed", now.Sub(entry.last_used).
				Round(time.Second).String()).
			Set("Age", now.Sub(entry.last_refresh).
				Round(time.Second).String()).
			Set("LastRefresh", now.Sub(last_refresh).
				Round(time.Second).String())
	}
}

func MaybeInstallDNSCache(ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if config_obj.Client == nil ||
		config_obj.Client.DnsCacheRefreshMin == 0 {
		return nil
	}

	timeout := time.Duration(config_obj.Client.DnsCacheRefreshMin) * time.Minute

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.Info("<green>Starting</> Local DNSCache with refresh of %v",
		timeout.String())

	res := &CachingResolver{
		cache:   make(map[string]*cacheEntry),
		Timeout: timeout,
		ctx:     ctx,
	}
	DefaultResolver = res

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(utils.Jitter(timeout)):
				res.RefreshOnce()
			}
		}
	}()

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "DNS Cache",
		Description:   "A global DNS cache.",
		ProfileWriter: res.WriteProfile,
		Categories:    []string{"Client"},
	})

	return nil
}
