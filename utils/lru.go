// Based on
// https://github.com/hashicorp/golang-lru/blob/master/simplelru/lru.go
// but more optimized for speed: by changing keys to int mapaccess is
// much faster. We also added locking to the LRU to make it thread
// safe.

package utils

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
)

// EvictCallback is used to get a callback when a cache entry is evicted
type EvictCallback func(key int, value interface{})

// LRU implements a thread safe fixed size LRU cache
type LRU struct {
	size      int
	evictList *list.List
	items     map[int]*list.Element
	onEvict   EvictCallback

	mu sync.Mutex

	name  string
	hits  int64
	miss  int64
	total int64
}

// entry is used to hold a value in the evictList
type entry struct {
	key   int
	value interface{}
}

// NewLRU constructs an LRU of the given size
func NewLRU(size int, onEvict EvictCallback, name string) (*LRU, error) {
	if size <= 0 {
		return nil, errors.New("Must provide a positive size")
	}
	c := &LRU{
		size:      size,
		evictList: list.New(),
		items:     make(map[int]*list.Element),
		onEvict:   onEvict,
		name:      name,
	}
	return c, nil
}

// Purge is used to completely clear the cache.
func (self *LRU) Purge() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for k, v := range self.items {
		if self.onEvict != nil {
			self.onEvict(k, v.Value.(*entry).value)
		}
		delete(self.items, k)
	}
	self.evictList.Init()
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (self *LRU) Add(key int, value interface{}) (evicted bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.total++

	// Check for existing item
	if ent, ok := self.items[key]; ok {
		self.evictList.MoveToFront(ent)
		ent.Value.(*entry).value = value
		return false
	}

	// Add new item
	ent := &entry{key, value}
	entry := self.evictList.PushFront(ent)
	self.items[key] = entry

	evict := self.evictList.Len() > self.size
	// Verify size not exceeded
	if evict {
		self.removeOldest()
	}
	return evict
}

func (self *LRU) Touch(key int) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if ent, ok := self.items[key]; ok {
		self.evictList.MoveToFront(ent)
	}
}

// Get looks up a key's value from the cache.
func (self *LRU) Get(key int) (value interface{}, ok bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if ent, ok := self.items[key]; ok {
		self.evictList.MoveToFront(ent)
		self.hits++
		return ent.Value.(*entry).value, true
	}
	self.miss++
	return
}

// Contains checks if a key is in the cache, without updating the recent-ness
// or deleting it for being stale.
func (self *LRU) Contains(key int) (ok bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	_, ok = self.items[key]
	return ok
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (self *LRU) Peek(key int) (value interface{}, ok bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	var ent *list.Element
	if ent, ok = self.items[key]; ok {
		return ent.Value.(*entry).value, true
	}
	return nil, ok
}

// Remove removes the provided key from the cache, returning if the
// key was contained.
func (self *LRU) Remove(key int) (present bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if ent, ok := self.items[key]; ok {
		self.removeElement(ent)
		return true
	}
	return false
}

// RemoveOldest removes the oldest item from the cache.
func (self *LRU) RemoveOldest() (key int, value interface{}, ok bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	ent := self.evictList.Back()
	if ent != nil {
		self.removeElement(ent)
		kv := ent.Value.(*entry)
		return kv.key, kv.value, true
	}
	return 0, nil, false
}

// GetOldest returns the oldest entry
func (self *LRU) GetOldest() (key int, value interface{}, ok bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	ent := self.evictList.Back()
	if ent != nil {
		kv := ent.Value.(*entry)
		return kv.key, kv.value, true
	}
	return 0, nil, false
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (self *LRU) Keys() []int {
	self.mu.Lock()
	defer self.mu.Unlock()

	keys := make([]int, len(self.items))
	i := 0
	for ent := self.evictList.Back(); ent != nil; ent = ent.Prev() {
		keys[i] = ent.Value.(*entry).key
		i++
	}
	return keys
}

// Len returns the number of items in the cache.
func (self *LRU) Len() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.evictList.Len()
}

func (self *LRU) DebugString() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return fmt.Sprintf("%s LRU %p hit %d miss %d - total %v (%f)\n",
		self.name, self,
		self.hits, self.miss, self.total,
		float64(self.hits)/float64(self.miss))
}

// removeOldest removes the oldest item from the cache.
func (self *LRU) removeOldest() {
	ent := self.evictList.Back()
	if ent != nil {
		self.removeElement(ent)
	}
}

// removeElement is used to remove a given list element from the cache
func (self *LRU) removeElement(e *list.Element) {
	self.evictList.Remove(e)
	kv := e.Value.(*entry)
	delete(self.items, kv.key)
	if self.onEvict != nil {
		self.onEvict(kv.key, kv.value)
	}
}
