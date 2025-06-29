//go:build !arm && !mips && !(linux && 386)
// +build !arm
// +build !mips
// +build !linux !386

package pst

import (
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	pst "github.com/mooijtech/go-pst/v6/pkg"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/files"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	PSTCacheTag = "_PST_CACHE"
)

var (
	InUseError = errors.New("In Use")
)

type PSTFile struct {
	*pst.File

	// Reference count reader
	mu     sync.Mutex
	refs   int
	reader accessors.ReadSeekCloser

	// Maintain an index of everything for fast access.
	paths       map[pst.Identifier]string
	attachments map[pst.Identifier]*pst.Attachment

	// For tracking file opens
	key string
}

func (self *PSTFile) GetPath(id pst.Identifier) string {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, _ := self.paths[id]
	return res
}

func (self *PSTFile) setPath(parent_id, id pst.Identifier, name string) {
	root, _ := self.paths[parent_id]
	new_path := path.Join(root, name)
	self.paths[id] = new_path
}

func (self *PSTFile) walkFolders(folder *pst.Folder) error {
	subFolders, err := folder.GetSubFolders()
	if err != nil {
		return err
	}

	for _, subFolder := range subFolders {
		self.setPath(folder.Identifier, subFolder.Identifier, subFolder.Name)

		// Walk the children
		err := self.walkFolders(&subFolder)
		if err != nil {
			return err
		}

		messageIterator, err := subFolder.GetMessageIterator()
		if err != nil {
			continue
		}

		for messageIterator.Next() {
			message := messageIterator.Value()
			self.setPath(subFolder.Identifier, message.Identifier,
				fmt.Sprintf("Msg-%d", message.Identifier))

			attachmentIterator, err := message.GetAttachmentIterator()
			if err != nil {
				continue
			}

			for attachmentIterator.Next() {
				attachment := attachmentIterator.Value()
				self.setPath(message.Identifier, attachment.Identifier,
					fmt.Sprintf("Att-%d", attachment.Identifier))

				self.attachments[attachment.Identifier] = attachment
			}
		}

	}
	return nil
}

func (self *PSTFile) Initialize() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	rootFolder, err := self.GetRootFolder()
	if err != nil {
		return err
	}

	self.setPath(rootFolder.Identifier, rootFolder.Identifier, rootFolder.Name)
	return self.walkFolders(&rootFolder)
}

func (self *PSTFile) IncRef() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.refs++
}

func (self *PSTFile) ForceClose() {
	self.reader.Close()
}

func (self *PSTFile) TryToClose() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.refs != 0 {
		return InUseError
	}

	self.File.Cleanup()
	self.reader.Close()
	files.Remove(self.key)

	return nil
}

func (self *PSTFile) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.refs--
}

// Used to open a fixed attachement for reading.
func (self *PSTFile) GetAttachment(att_id pst.Identifier) (
	res *pst.Attachment, closer func(), err error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.attachments[att_id]
	if !pres {
		return nil, nil, utils.NotFoundError
	}
	self.refs++

	// Keep track of open files.
	key := fmt.Sprintf("%v-%d", self.key, att_id)
	files.Add(key)

	return res, func() {
		self.Close()
		files.Remove(key)
	}, nil
}

type PSTCache struct {
	mu sync.Mutex

	// key: Accessor/Pathspec Value: PSTFile
	lru *ttlcache.Cache
}

func (self *PSTCache) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.lru.Close()
}

// Opens the PST file or get it from cache.
func (self *PSTCache) Open(
	scope vfilter.Scope,
	accessor_name string, path *accessors.OSPath) (*PSTFile, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	key := accessor_name + path.String()
	pst_file_any, err := self.lru.Get(key)
	// Cache hit
	if err == nil {
		res := pst_file_any.(*PSTFile)
		res.IncRef()
		return res, nil
	}

	// Open it the old fasioned way.
	accessor, err := accessors.GetAccessor(accessor_name, scope)
	if err != nil {
		return nil, err
	}

	reader, err := accessor.OpenWithOSPath(path)
	if err != nil {
		return nil, err
	}
	files.Add(key)

	// Closed by the PSTFile when refs are zero and cache timeout is
	// reached.

	pstFile, err := pst.New(utils.MakeReaderAtter(reader))
	if err != nil {
		return nil, err
	}

	// Cache the file
	res := &PSTFile{
		File:        pstFile,
		reader:      reader,
		attachments: make(map[pst.Identifier]*pst.Attachment),
		paths:       make(map[pst.Identifier]string),
		key:         key,
	}
	err = res.Initialize()
	if err != nil {
		return nil, err
	}

	res.IncRef()

	return res, self.lru.Set(key, res)
}

func GetPSTCache(scope vfilter.Scope) *PSTCache {
	cache, ok := vql_subsystem.CacheGet(scope, PSTCacheTag).(*PSTCache)
	if ok {
		return cache
	}

	cache_size := int(vql_subsystem.GetIntFromRow(
		scope, scope, constants.PST_CACHE_SIZE))
	if cache_size == 0 {
		cache_size = 20
	}

	// Cache is disabled.
	if cache_size < 0 {
		return &PSTCache{}
	}

	cache_time := vql_subsystem.GetIntFromRow(
		scope, scope, constants.PST_CACHE_TIME)
	if cache_time == 0 {
		cache_time = 60
	}

	cache = &PSTCache{
		lru: ttlcache.NewCache(),
	}

	cache.lru.SetCacheSizeLimit(cache_size)
	_ = cache.lru.SetTTL(time.Second * time.Duration(cache_time))

	cache.lru.SetExpirationCallback(
		func(key string, value interface{}) error {
			ctx, ok := value.(*PSTFile)
			if ok {
				// Do not block the lru while closing.
				go ctx.Close()
			}
			return nil
		})

	root_scope := vql_subsystem.GetRootScope(scope)
	_ = root_scope.AddDestructor(func() {
		cache.Close()
	})
	vql_subsystem.CacheSet(root_scope, PSTCacheTag, cache)

	return cache
}
