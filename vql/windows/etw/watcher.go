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

	registrations map[string][]*Handle
	sessions      map[string]*etw.Session
}

func NewEventTraceWatcherService() *EventTraceWatcherService {
	return &EventTraceWatcherService{
		registrations: make(map[string][]*Handle),
		sessions:      make(map[string]*etw.Session),
	}
}

func (self *EventTraceWatcherService) Register(
	ctx context.Context,
	scope vfilter.Scope,
	provider_guid string,
	session_name string,
	any_keyword uint64, all_keyword uint64, level int64,
	wGuid windows.GUID,
	output_chan chan vfilter.Row) (func(), error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	subctx, cancel := context.WithCancel(ctx)

	handle := &Handle{
		ctx:         subctx,
		output_chan: output_chan,
		scope:       scope,
		guid:        wGuid}

	registration, pres := self.registrations[session_name]
	if !pres {
		registration = []*Handle{}
		self.registrations[session_name] = registration
	}

	session, pres := self.sessions[session_name]
	if !pres {
		session, err := self.createSession(ctx, scope, session_name, output_chan)
		if err != nil {
			return cancel, err
		}

		err = self.UpdateSession(
			ctx, scope, session, wGuid, session_name,
			any_keyword, all_keyword, level)
		if err != nil {
			return cancel, err
		}

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
			subctx, subscope, session, session_name, frequency, cancel)
	} else {
		err := self.UpdateSession(
			ctx, scope, session, wGuid, session_name,
			any_keyword, all_keyword, level)
		if err != nil {
			return cancel, err
		}
	}

	registration = append(registration, handle)
	self.registrations[session_name] = registration

	scope.Log("Registering watcher for %v", provider_guid)

	return cancel, nil
}

// StartMonitoring monitors the session and emits events to all interested
// listeners. If no listeners exist we terminate.
func (self *EventTraceWatcherService) StartMonitoring(
	ctx context.Context, scope vfilter.Scope, session *etw.Session,
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

		self.mu.Lock()
		handles, pres := self.registrations[key]
		self.mu.Unlock()

		// No more listeners left, we are done.
		if !pres || len(handles) == 0 {
			session.Close()
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
		err := session.Process(cb)
		if err != nil {
			scope.Log("watch_etw: %v", err)
		}
	}()
}

func (self *EventTraceWatcherService) createSession(
	ctx context.Context, scope types.Scope,
	session_name string,
	output_chan chan vfilter.Row) (*etw.Session, error) {

	session, err := etw.NewSession(session_name)
	if err != nil {
		scope.Log("watch_etw: %s", err.Error())

		// Try to kill the session and recreate it.
		err = etw.KillSession(session_name)
		if err != nil {
			return session, err
		}

		session, err = etw.NewSession(session_name)
		if err != nil {
			return session, err
		}

	}

	self.sessions[session_name] = session

	return session, nil
}

func (self *EventTraceWatcherService) UpdateSession(
	ctx context.Context, scope types.Scope,
	session *etw.Session, guid windows.GUID, session_name string,
	any_keyword uint64, all_keyword uint64, level int64) error {

	return session.UpdateOptions(guid, func(cfg *etw.SessionOptions) {
		cfg.MatchAnyKeyword = any_keyword
		cfg.MatchAllKeyword = all_keyword
		cfg.Level = etw.TraceLevel(level)
		cfg.Name = session_name
		cfg.Guid = guid
	})
}

func CloseSession(session_name string, provider_guid string) error {
	handles, pres := GlobalEventTraceService.registrations[session_name]
	if !pres {
		return nil // No Session to kill.
	}

	// Check if GUID is valid
	guid, err := windows.GUIDFromString(provider_guid)
	if err != nil {
		return err
	}

	// Remove our entry from the list of handles.
	for i, handle := range handles {
		if handle.guid == guid {
			handle.ctx.Done()
			handles = append(handles[:i], handles[i+1:]...)
		}
	}
	GlobalEventTraceService.registrations[session_name] = handles
	if len(handles) == 0 {
		return etw.KillSession(session_name)
	}
	return nil
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
