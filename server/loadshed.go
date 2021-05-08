package server

import (
	"net"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	loadshedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_loadshed_count",
		Help: "Number of connections rejected due to load shedding.",
	})
)

type LoadSheddingListener struct {
	net.Listener
	throttler *utils.Throttler
}

func (self *LoadSheddingListener) Accept() (net.Conn, error) {
	for {
		res, err := self.Listener.Accept()
		if err != nil {
			return res, err
		}

		if self.throttler == nil || self.throttler.Ready() {
			return res, err
		}

		// We are not ready to accept new connections, close
		// this one and try again later.
		loadshedCounter.Inc()
		res.Close()
	}
}

func (self *Server) NewLoadSheddingListener(addr string) (*LoadSheddingListener, error, func() error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err, nil
	}

	return &LoadSheddingListener{
		Listener:  ln,
		throttler: self.throttler,
	}, err, ln.Close
}
