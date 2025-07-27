package uploads

import "sync"

var (
	gClientUploaderTracker = ClientUploaderTracker{
		uploaders: make(map[uint64]*VelociraptorUploader),
	}
)

type ClientUploaderTracker struct {
	mu        sync.Mutex
	uploaders map[uint64]*VelociraptorUploader
}

func (self *ClientUploaderTracker) Register(
	id uint64, uploader *VelociraptorUploader) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.uploaders[id] = uploader
}

func (self *ClientUploaderTracker) Unregister(id uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.uploaders, id)
}
