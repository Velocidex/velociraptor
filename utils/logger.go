package utils

import (
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/vfilter"
)

type logCacheEntry struct {
	last_time time.Time
}

type DeduplicatedLogger struct {
	lru        *ttlcache.Cache // map[string]logCacheEntry
	dedup_time time.Duration
}

func (self *DeduplicatedLogger) Log(
	scope vfilter.Scope, message string, args ...interface{}) {

	// Do we need to dedup it?
	now := GetTime().Now()
	log_cache_entry_any, err := self.lru.Get(message)
	if err == nil {
		log_cache_entry, ok := log_cache_entry_any.(*logCacheEntry)
		if ok && log_cache_entry.last_time.After(now.Add(-self.dedup_time)) {
			return
		}
	}

	log_cache_entry := &logCacheEntry{
		last_time: now,
	}
	self.lru.Set(message, log_cache_entry)

	scope.Log(message, args...)
}

func NewDeduplicatedLogger(dedup_time time.Duration) (
	res *DeduplicatedLogger, closer func() error) {
	res = &DeduplicatedLogger{
		lru:        ttlcache.NewCache(),
		dedup_time: dedup_time,
	}

	res.lru.SetCacheSizeLimit(100)
	return res, res.lru.Close
}
