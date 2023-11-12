package etw

import (
	"context"
	"sync"

	"github.com/Velocidex/etw"
	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// A Session context manages an ETW session:

// 1. It can accomodate multiple registrations on multiple GUIDs.
// 2. It can also accomodate multiple watchers on the same GUID.
// 3. As GUIDs are added or removes the session is manipulated to add/remove providers.
// 4. When there are no more providers, we can close the session.
// 5. If there are newer registrations, we can open the session again.
type SessionContext struct {
	mu sync.Mutex

	name    string
	session *etw.Session

	wg sync.WaitGroup

	// Registrations keyed by GUID
	registrations map[string][]*Handle
}

// Handle is used to track all watchers. We write the event on to the
// output_chan unless the context is done. When all interested parties
// are done we may destroy the monitoring go routine and remove the
// registration.
type Handle struct {
	ctx         context.Context
	cancel      func()
	id          uint64
	output_chan chan vfilter.Row
	scope       vfilter.Scope
	guid        windows.GUID

	mu     sync.Mutex
	closed bool
}

func (self *Handle) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.closed = true

	self.cancel()
	close(self.output_chan)
}

// Try to send but skip closed handles.
func (self *Handle) Send(event vfilter.Row) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.closed {
		return
	}

	select {
	case <-self.ctx.Done():
		return
	case self.output_chan <- event:
	}
}

func NewHandle(ctx context.Context,
	scope vfilter.Scope, guid windows.GUID) *Handle {

	subctx, cancel := context.WithCancel(ctx)
	return &Handle{
		ctx:         subctx,
		cancel:      cancel,
		id:          utils.GetId(),
		output_chan: make(chan vfilter.Row),
		scope:       scope,
		guid:        guid,
	}
}

func (self *SessionContext) Session(scope vfilter.Scope) (*etw.Session, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._Session(scope)
}

func (self *SessionContext) _Session(scope vfilter.Scope) (*etw.Session, error) {
	// Cache the session for reuse.
	if self.session != nil {
		return self.session, nil
	}

	// Establish a new ETW session and cache it for next time.
	session, err := etw.NewSession(self.name, self.processEvent)
	if err != nil {

		// Try to kill the session and recreate it.
		err = etw.KillSession(self.name)
		if err != nil {
			return nil, err
		}

		session, err = etw.NewSession(self.name, self.processEvent)
		if err != nil {
			return nil, err
		}
	}

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()

		err := session.Process()
		if err != nil {
			scope.Log("etw: Can not start session %v: %v", self.name, err)
		}
	}()

	self.session = session

	return self.session, nil
}

func (self *SessionContext) processEvent(e *etw.Event) {
	defer utils.CheckForPanic("processEvent")

	event_guid := e.Header.ProviderID.String()

	self.mu.Lock()
	registrations, pres := self.registrations[event_guid]
	if !pres {
		self.mu.Unlock()
		return
	}

	handlers := append([]*Handle{}, registrations...)
	self.mu.Unlock()

	event := ordereddict.NewDict().
		Set("System", e.Header).
		Set("ProviderGUID", event_guid)

	data, err := e.EventProperties()
	if err == nil {
		event.Set("EventData", data)
	}

	// Send the event to all interested parties.
	for _, handle := range handlers {
		if handle.guid == e.Header.ProviderID {
			handle.Send(event)
		}
	}
}

func (self *SessionContext) Stats() []ProviderStat {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []ProviderStat{}
	for guid, registrations := range self.registrations {
		result = append(result, ProviderStat{
			GUID:     guid,
			Watchers: len(registrations),
		})
	}

	return result
}

// Callers call this to register a watcher on the GUID
func (self *SessionContext) Register(
	ctx context.Context,
	scope vfilter.Scope,
	any_keyword uint64, all_keyword uint64, level int64,
	guid windows.GUID) (closer func(), output_chan chan vfilter.Row, err error) {

	key := guid.String()
	handle := NewHandle(ctx, scope, guid)

	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, pres := self.registrations[key]
	if !pres {
		// No one is currently watching this GUID, lets begin watching
		// it.
		session, err := self._Session(scope)
		if err != nil {
			return nil, nil, err
		}

		err = session.SubscribeToProvider(etw.SessionOptions{
			Guid:            guid,
			Level:           etw.TraceLevel(level),
			MatchAnyKeyword: any_keyword,
			MatchAllKeyword: all_keyword,
		})
		if err != nil {
			scope.Log("etw: Can not add provider to session %v: %v", self.name, err)
			return nil, nil, err
		}
		scope.Log("etw: Added provider %v to session %v", guid.String(), self.name)
	}

	registrations = append(registrations, handle)
	self.registrations[key] = registrations

	return func() {
		self.DeregisterHandle(key, handle.id, guid, scope)
	}, handle.output_chan, nil
}

// Remove the handle from the set of registrations
func (self *SessionContext) DeregisterHandle(
	key string, id uint64, guid windows.GUID, scope vfilter.Scope) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, pres := self.registrations[key]
	if !pres {
		// No registrations for this provider
		return
	}

	// Remove the handle from the registrations
	new_reg := make([]*Handle, 0, len(registrations))
	for _, handle := range registrations {
		if handle.id == id {
			scope.Log("etw: Deregistering %v from session %v",
				handle.guid.String(), self.name)
			handle.Close()

		} else {
			// Retain other handles
			new_reg = append(new_reg, handle)
		}
	}

	if len(new_reg) > 0 {
		self.registrations[key] = new_reg
		return
	}

	// No more registrations for this GUID. We can remove the provider
	// from the ETW session.
	delete(self.registrations, key)
	scope.Log("etw: Removing provider %v from session %v",
		guid.String(), self.name)

	session, err := self._Session(scope)
	if err == nil {
		err := session.UnsubscribeFromProvider(guid)
		if err != nil {
			scope.Log("ERROR:etw: failed to disable provider; %w", err)
		}
	}

	// No providers left, kill the session.
	if len(self.registrations) == 0 {
		if self.session != nil {
			scope.Log("etw: Closing session %v", self.name)
			self.session.Close()
		}
		self.session = nil
	}

}
