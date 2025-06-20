package networking

import (
	"context"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	gConnectionTracker *ConnectionTracker
)

type TrackedConnection struct {
	net.Conn

	id                    int64
	localAddr, remoteAddr net.Addr
	dns                   string
	Created               int64
	Closed                int64
	ReadCount, WriteCount int64
}

func (self *TrackedConnection) Read(b []byte) (n int, err error) {
	n, err = self.Conn.Read(b)
	atomic.AddInt64(&self.ReadCount, int64(n))
	return n, err
}

func (self *TrackedConnection) Write(b []byte) (n int, err error) {
	n, err = self.Conn.Write(b)
	atomic.AddInt64(&self.WriteCount, int64(n))
	return n, err
}

func (self *TrackedConnection) Close() error {
	atomic.StoreInt64(&self.Closed, utils.GetTime().Now().UnixNano())
	return self.Conn.Close()
}

type ConnectionTracker struct {
	mu    sync.Mutex
	conns map[int64]*TrackedConnection
}

func (self *ConnectionTracker) reap() {
	now := utils.GetTime().Now()
	for id, t := range self.conns {
		closed := atomic.LoadInt64(&t.Closed)
		if closed > 0 {
			closed_time := time.Unix(0, closed)
			if now.Sub(closed_time) > time.Duration(time.Minute*1) {
				delete(self.conns, id)
			}
		}
	}
}

func (self *ConnectionTracker) NewTrackedConnection(conn net.Conn, dns string) net.Conn {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.reap()

	res := &TrackedConnection{
		id:         utils.GetGUID(),
		Conn:       conn,
		Created:    utils.GetTime().Now().UnixNano(),
		localAddr:  conn.LocalAddr(),
		remoteAddr: conn.RemoteAddr(),
		dns:        dns,
	}
	self.conns[res.id] = res
	return res
}

func (self *ConnectionTracker) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	var rows []*ordereddict.Dict
	self.mu.Lock()

	self.reap()

	for _, t := range self.conns {
		created := time.Unix(0, atomic.LoadInt64(&t.Created))
		now := utils.GetTime().Now()

		closed := "active"
		closed_ago := ""
		if t.Closed > 0 {
			closed_time := time.Unix(0, atomic.LoadInt64(&t.Closed))
			closed = closed_time.Format(time.RFC3339)
			closed_ago = now.Sub(closed_time).Round(time.Second).String()
		}

		rows = append(rows, ordereddict.NewDict().
			Set("Created", created.Format(time.RFC3339)).
			Set("CreatedAgo", now.Sub(created).Round(time.Second).String()).
			Set("Closed", closed).
			Set("ClosedAgo", closed_ago).
			Set("LocalAddr", t.localAddr.String()).
			Set("RemoteAddr", t.remoteAddr.String()).
			Set("RemoteName", t.dns).
			Set("Read", atomic.LoadInt64(&t.ReadCount)).
			Set("Write", atomic.LoadInt64(&t.WriteCount)))
	}
	self.mu.Unlock()

	sort.Slice(rows, func(i, j int) bool {
		created_i, _ := rows[i].GetString("Created")
		created_j, _ := rows[j].GetString("Created")
		return created_j < created_i
	})

	for _, row := range rows {
		select {
		case <-ctx.Done():
			return
		case output_chan <- row:
		}
	}
}

func init() {
	gConnectionTracker = &ConnectionTracker{
		conns: make(map[int64]*TrackedConnection),
	}

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "HTTP Connections",
		Description:   "Tracks HTTP connections made by the process",
		ProfileWriter: gConnectionTracker.WriteProfile,
		Categories:    []string{"Global"},
	})

}
