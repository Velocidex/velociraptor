package rsyslog

import (
	"time"

	"github.com/Velocidex/ttlcache/v2"
)

type connectionPool struct {
	lru *ttlcache.Cache // map[string]*Logger
}

func NewConnectionPool() *connectionPool {
	self := &connectionPool{
		lru: ttlcache.NewCache(),
	}

	self.lru.SetTTL(time.Minute)
	self.lru.SetCacheSizeLimit(10)
	self.lru.SetExpirationCallback(func(key string, value interface{}) error {
		logger, ok := value.(*Logger)
		if ok {
			logger.Close()
		}
		return nil
	})

	return self
}
