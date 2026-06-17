package locker

import (
	"sync"
	"sync/atomic"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

// A Path locker implements serialization for writers on filesystem
// paths. Datastore implementations can use it to ensure files are
// locked during writes and can be accessed by multiple writers
// safely.
type lockWriteWrapper struct {
	api.FileWriter

	mu     sync.Mutex
	locker *lockerHandle
}

func (self *lockWriteWrapper) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Make sure we only call Close() exactly once
	if self.locker == nil {
		return nil
	}

	err := self.FileWriter.Close()
	self.locker.Close()
	self.locker = nil
	return err
}

type lockerHandle struct {

	// How many callers are waiting for the lock.  When we close the
	// lock and no callers are waiting then we can remove it from the
	// path locker.
	pending int64

	mu         sync.Mutex
	lock_count int

	key   string
	owner *PathLocker
}

func (self *lockerHandle) lock() {
	self.mu.Lock()
	self.lock_count++
}

func (self *lockerHandle) Close() {
	self.lock_count--
	if self.lock_count == 0 {
		self.owner.MaybeReap(self)
		self.mu.Unlock()
	}
}

func (self *lockerHandle) WrapWriter(in api.FileWriter) api.FileWriter {
	self.lock_count++
	return &lockWriteWrapper{
		FileWriter: in,
		locker:     self,
	}
}

type PathLocker struct {
	mu          sync.Mutex
	in_progress map[string]*lockerHandle
}

type PathLockerStats struct {
	InProgress int
	Files      []string
}

func NewPathLocker() *PathLocker {
	return &PathLocker{
		in_progress: make(map[string]*lockerHandle),
	}
}

func (self *PathLocker) Stats() *PathLockerStats {
	self.mu.Lock()
	defer self.mu.Unlock()

	res := &PathLockerStats{
		InProgress: len(self.in_progress),
	}

	for k := range self.in_progress {
		res.Files = append(res.Files, k)
		if len(res.Files) == 100 {
			break
		}
	}
	return res
}

func (self *PathLocker) MaybeReap(locker *lockerHandle) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Decrement the pending count for this locker and remove it from
	// the map if there are no more writers pending.
	pending := atomic.AddInt64(&locker.pending, -1)
	if pending == 0 {
		delete(self.in_progress, locker.key)
	}
}

// Gets a handle for locking. The handle will be locked when this function returns.
// The handle must either be
func (self *PathLocker) GetHandle(filename api.FSPathSpec) *lockerHandle {
	self.mu.Lock()

	key := filename.AsClientPath()
	res, pres := self.in_progress[key]
	if pres {
		// Increase the pending count before we take the lock. The
		// pending count indicates how many goroutines are waiting on
		// the lock.
		atomic.AddInt64(&res.pending, 1)
		self.mu.Unlock()

		res.lock()
		return res
	}

	res = &lockerHandle{
		key:     key,
		owner:   self,
		pending: 1,
	}
	self.in_progress[key] = res

	self.mu.Unlock()
	res.lock()

	return res
}
