// Based on
// https://github.com/hashicorp/golang-lru/blob/master/simplelru/lru.go
// but more optimized for speed: by changing keys to int mapaccess is
// much faster. We also added locking to the LRU to make it thread
// safe.

package utils

import (
	"container/list"
	"errors"
	"hash/fnv"
	"sync"
)

// A Quick way to get an integer hash for use in strings.
func GetLRUHash(str string) int {
	h := fnv.New64()
	h.Write([]byte(str))
	return int(h.Sum64())
}

// EvictCallback is used to get a callback when a cache entry is evicted
type EvictCallback func(key int, value interface{})

// LRU implements a thread safe fixed size LRU cache
type LRU struct {
	size      int
	evictList *list.List
	items     map[int]*list.Element
	onEvict   EvictCallback

	mu sync.Mutex
}

// entry is used to hold a value in the evictList
type entry struct {
	key   int
	value interface{}
}

// NewLRU constructs an LRU of the given size
func NewLRU(size int, onEvict EvictCallback) (*LRU, error) {
	if size <= 0 {
		return nil, errors.New("Must provide a positive size")
	}
	c := &LRU{
		size:      size,
		evictList: list.New(),
		items:     make(map[int]*list.Element),
		onEvict:   onEvict,
	}
	return c, nil
}

// Purge is used to completely clear the cache.
func (c *LRU) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range c.items {
		if c.onEvict != nil {
			c.onEvict(k, v.Value.(*entry).value)
		}
		delete(c.items, k)
	}
	c.evictList.Init()
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (c *LRU) Add(key int, value interface{}) (evicted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check for existing item
	if ent, ok := c.items[key]; ok {
		c.evictList.MoveToFront(ent)
		ent.Value.(*entry).value = value
		return false
	}

	// Add new item
	ent := &entry{key, value}
	entry := c.evictList.PushFront(ent)
	c.items[key] = entry

	evict := c.evictList.Len() > c.size
	// Verify size not exceeded
	if evict {
		c.removeOldest()
	}
	return evict
}

// Get looks up a key's value from the cache.
func (c *LRU) Get(key int) (value interface{}, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ent, ok := c.items[key]; ok {
		c.evictList.MoveToFront(ent)
		return ent.Value.(*entry).value, true
	}
	return
}

// Contains checks if a key is in the cache, without updating the recent-ness
// or deleting it for being stale.
func (c *LRU) Contains(key int) (ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok = c.items[key]
	return ok
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (c *LRU) Peek(key int) (value interface{}, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var ent *list.Element
	if ent, ok = c.items[key]; ok {
		return ent.Value.(*entry).value, true
	}
	return nil, ok
}

// Remove removes the provided key from the cache, returning if the
// key was contained.
func (c *LRU) Remove(key int) (present bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ent, ok := c.items[key]; ok {
		c.removeElement(ent)
		return true
	}
	return false
}

// RemoveOldest removes the oldest item from the cache.
func (c *LRU) RemoveOldest() (key int, value interface{}, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
		kv := ent.Value.(*entry)
		return kv.key, kv.value, true
	}
	return 0, nil, false
}

// GetOldest returns the oldest entry
func (c *LRU) GetOldest() (key int, value interface{}, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ent := c.evictList.Back()
	if ent != nil {
		kv := ent.Value.(*entry)
		return kv.key, kv.value, true
	}
	return 0, nil, false
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *LRU) Keys() []int {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]int, len(c.items))
	i := 0
	for ent := c.evictList.Back(); ent != nil; ent = ent.Prev() {
		keys[i] = ent.Value.(*entry).key
		i++
	}
	return keys
}

// Len returns the number of items in the cache.
func (c *LRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.evictList.Len()
}

// removeOldest removes the oldest item from the cache.
func (c *LRU) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement is used to remove a given list element from the cache
func (c *LRU) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	kv := e.Value.(*entry)
	delete(c.items, kv.key)
	if c.onEvict != nil {
		c.onEvict(kv.key, kv.value)
	}
}
