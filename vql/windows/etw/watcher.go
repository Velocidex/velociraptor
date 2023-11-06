//go:build windows && cgo
// +build windows,cgo

package etw

import (
	"context"
	"sync"

	"github.com/Velocidex/etw"
	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	GlobalEventTraceService = NewEventTraceWatcherService()
)

// EventTraceWatcherService watches one or more event ETW sessions and multiplexes
// events to multiple readers.
type EventTraceWatcherService struct {
	mu sync.Mutex

	sessions map[string]*SessionContext
}

func NewEventTraceWatcherService() *EventTraceWatcherService {
	return &EventTraceWatcherService{
		sessions: make(map[string]*SessionContext),
	}
}

// Register returns a channel.

func (self *EventTraceWatcherService) Register(
	ctx context.Context,
	scope vfilter.Scope,
	provider_guid string,
	session_name string,
	any_keyword uint64, all_keyword uint64, level int64,
	wGuid windows.GUID) (func(), chan vfilter.Row, error) {

	self.mu.Lock()
	defer self.mu.Unlock()
	subctx, cancel := context.WithCancel(ctx)

	output_chan := make(chan vfilter.Row)

	handle := &Handle{
		ctx:         subctx,
		output_chan: output_chan,
		scope:       scope,
		guid:        wGuid}

	deregistration := func() {
		cancel()

		scope.Log("Unregistering %v\n", handle.guid)

		self.mu.Lock()
		sessionContext, pres := self.sessions[session_name]
		self.mu.Unlock()
		if !pres {
			return
		}

		sessionContext.mu.Lock()
		defer sessionContext.mu.Unlock()

		new_reg := make([]*Handle, len(sessionContext.registrations))
		for _, handle := range sessionContext.registrations {
			if handle.guid != wGuid {
				new_reg = append(new_reg, handle)
			}
		}

		if len(new_reg) == 0 {
			sessionContext.Close()
			delete(self.sessions, session_name)
		} else {
			sessionContext.registrations = new_reg
		}
	}

	// Check if we already have a session for this provider.
	sessionContext, pres := self.sessions[session_name]
	if !pres {
		var err error
		sessionContext, err = self.NewSessionContext(session_name, scope)
		if err != nil {
			return cancel, nil, err
		}

		// err = sessionContext.UpdateSession(ctx, scope, wGuid, any_keyword, all_keyword, level)
		// if err != nil {
		// 	return cancel, nil, err
		// }

		// Create a scope with a completely different lifespan since
		// it may outlive this query (if another query starts watching
		// the same file). The query will inherit the same ACL
		// manager, log manager etc but this is usually fine as there
		// are not different ACLs managers on the client side.
		manager := &repository.RepositoryManager{}
		builder := services.ScopeBuilderFromScope(scope)
		subscope := manager.BuildScope(builder)
		frequency := vql_subsystem.GetIntFromRow(
			scope, scope, constants.EVTX_FREQUENCY)

		go self.StartMonitoring(
			subctx, subscope, sessionContext, session_name, frequency, cancel)

	}

	err := sessionContext.UpdateSession(
		ctx, scope, wGuid, any_keyword,
		all_keyword, level)
	if err != nil {
		return cancel, output_chan, err
	}

	scope.Log("Registering watcher for %v", provider_guid)
	sessionContext.mu.Lock()
	handles := sessionContext.registrations
	sessionContext.registrations = append(handles, handle)
	sessionContext.mu.Unlock()

	return deregistration, output_chan, nil
}

// StartMonitoring monitors the session and emits events to all interested
// listeners. If no listeners exist we terminate.
func (self *EventTraceWatcherService) StartMonitoring(
	ctx context.Context, scope vfilter.Scope, sessionContext *SessionContext,
	key string, frequency uint64, cancel context.CancelFunc) {

	defer utils.CheckForPanic("StartMonitoring")

	// By default check every 15 seconds. Event logs are not flushed
	// that often so checking more frequently does not help much.
	if frequency == 0 {
		frequency = 15
	}

	cb := func(e *etw.Event) {
		event := ordereddict.NewDict().
			Set("System", e.Header).
			Set("ProviderGUID", e.Header.ProviderID.String())

		data, err := e.EventProperties()
		if err == nil {
			event.Set("EventData", data)
		}

		if !sessionContext.status {
			return
		}

		sessionContext.mu.Lock()
		handles := sessionContext.registrations
		sessionContext.mu.Unlock()

		// No more listeners left, we are done.
		if len(handles) == 0 || handles[0] == nil {
			sessionContext.Close()
			return
		}

		// Send the event to all interested parties.
		for _, handle := range handles {
			if handle.guid == e.Header.ProviderID {
				select {
				case <-ctx.Done():
					return
				case handle.output_chan <- event:
				}
			}
		}
	}

	go func() {
		// When session.Process() exits, we exit the
		// query.
		err := sessionContext.session.Process(cb)
		if err != nil {
			scope.Log("watch_etw: %v", err)
		}
	}()
}

type SessionContext struct {
	mu sync.Mutex

	name          string
	status        bool
	session       *etw.Session
	registrations []*Handle
}

func (self *SessionContext) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.status = false

	// Empty the registrations and close the session.
	self.registrations = []*Handle{}
	self.session.Close()
	delete(GlobalEventTraceService.sessions, self.name)
}

func (self *EventTraceWatcherService) NewSessionContext(name string, scope vfilter.Scope) (*SessionContext, error) {

	sessionContext := &SessionContext{
		name:          name,
		registrations: []*Handle{},
		status:        true,
	}

	err := sessionContext.createSession()
	if err != nil {
		scope.Log("NewSessionContext: Failed to create session: %v", err)
		return nil, err
	}
	scope.Log("NewSessionContext: Created session %v", name)

	self.sessions[name] = sessionContext

	return sessionContext, nil
}

func (self *SessionContext) createSession() error {

	session, err := etw.NewSession(self.name)
	if err != nil {

		// Try to kill the session and recreate it.
		err = etw.KillSession(self.name)
		if err != nil {
			return err
		}

		session, err = etw.NewSession(self.name)
		if err != nil {
			return err
		}

	}

	self.session = session

	return nil
}

func (self *SessionContext) UpdateSession(
	ctx context.Context, scope types.Scope,
	guid windows.GUID, any_keyword uint64,
	all_keyword uint64, level int64) error {

	return self.session.UpdateOptions(guid, func(cfg *etw.SessionOptions) {
		cfg.MatchAnyKeyword = any_keyword
		cfg.MatchAllKeyword = all_keyword
		cfg.Level = etw.TraceLevel(level)
		cfg.Name = self.name
		cfg.Guid = guid
	})
}

// Handle is given for each interested party. We write the event on
// to the output_chan unless the context is done. When all interested
// parties are done we may destroy the monitoring go routine and remove
// the registration.
type Handle struct {
	ctx         context.Context
	output_chan chan vfilter.Row
	scope       vfilter.Scope
	guid        windows.GUID
}
