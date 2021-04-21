package client

import (
	"container/list"
	"sync"
)

type entry struct {
	source          string
	inbound_cipher  *_Cipher
	outbound_cipher *_Cipher
}

type CipherLRU struct {
	mu sync.Mutex

	list              *list.List
	by_source         map[string]*list.Element
	by_inbound_cipher map[string]*_Cipher

	size, capacity int64
}

// NewLRUCache creates a new empty cache with the given capacity.
func NewCipherLRU(capacity int64) *CipherLRU {
	return &CipherLRU{
		list:              list.New(),
		by_source:         make(map[string]*list.Element),
		by_inbound_cipher: make(map[string]*_Cipher),
		capacity:          capacity,
	}
}

func (self *CipherLRU) Clear() {
	self.size = 0
	self.list = list.New()
	self.by_source = make(map[string]*list.Element)
	self.by_inbound_cipher = make(map[string]*_Cipher)
}

// Get returns a value from the cache, and marks the entry as most
// recently used.
func (self *CipherLRU) GetByInboundCipher(enc_cipher []byte) (*_Cipher, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	cipher, pres := self.by_inbound_cipher[string(enc_cipher)]
	if !pres {
		return nil, false
	}
	self.moveSourceToFront(cipher.source)
	return cipher, true
}

func (self *CipherLRU) GetOutboundCipher(destination string) (*_Cipher, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	element, pres := self.by_source[destination]
	if !pres {
		return nil, false
	}

	old_entry := element.Value.(*entry)
	if old_entry.outbound_cipher == nil {
		return nil, false
	}

	self.moveSourceToFront(destination)
	return old_entry.outbound_cipher, true
}

func (self *CipherLRU) Set(source string, inbound_cipher, output_cipher *_Cipher) {
	self.mu.Lock()
	defer self.mu.Unlock()

	new_entry := &entry{
		source:          source,
		inbound_cipher:  inbound_cipher,
		outbound_cipher: output_cipher,
	}

	element, pres := self.by_source[source]
	if pres {
		old_entry := element.Value.(*entry)
		if new_entry.outbound_cipher == nil {
			new_entry.outbound_cipher = old_entry.outbound_cipher
		}

		if new_entry.inbound_cipher == nil {
			new_entry.inbound_cipher = old_entry.inbound_cipher
		}

		// update in place
		element.Value = new_entry

	} else {
		// Add new element
		element := self.list.PushFront(new_entry)
		self.by_source[source] = element
		if inbound_cipher != nil {
			self.by_inbound_cipher[string(
				inbound_cipher.encrypted_cipher)] = inbound_cipher
		}

		self.size++
		self.checkCapacity()
	}
}

// Delete cached keys to the source
func (self *CipherLRU) Delete(source string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	del_elem, pres := self.by_source[source]
	if !pres {
		return
	}

	self._delete(del_elem)
}

func (self *CipherLRU) _delete(del_elem *list.Element) {
	del_entry := del_elem.Value.(*entry)
	self.list.Remove(del_elem)

	delete(self.by_source, del_entry.source)

	if del_entry.inbound_cipher != nil {
		delete(self.by_inbound_cipher,
			string(del_entry.inbound_cipher.encrypted_cipher))
	}
	self.size--
}

func (self *CipherLRU) checkCapacity() {
	// Partially duplicated from Delete
	for self.size > self.capacity {
		del_elem := self.list.Back()
		self._delete(del_elem)
	}
}

func (self *CipherLRU) moveSourceToFront(source string) {
	element, pres := self.by_source[source]
	if pres {
		self.list.MoveToFront(element)
	}
}
