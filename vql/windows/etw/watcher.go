//go:build windows && cgo && amd64
// +build windows,cgo,amd64

package etw

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/vfilter"
)

var (
	GlobalEventTraceService = NewEventTraceWatcherService()
)

// EventTraceWatcherService watches one or more event ETW sessions and
// multiplexes events to multiple readers.
type EventTraceWatcherService struct {
	mu sync.Mutex

	// Map session names to session contexts - Each session can watch
	// multiple GUIDs, and there can be multiple watchers on the same
	// GUID. This is all managed by the session context. At the global
	// level we just multiplex by session names.
	sessions map[string]*SessionContext
}

func NewEventTraceWatcherService() *EventTraceWatcherService {
	return &EventTraceWatcherService{
		sessions: make(map[string]*SessionContext),
	}
}

// Register returns a channel where readers can receive events. When a
// reader is no longer interested in the data, they should call the
// closer function or cancel the ctx and the channel will be closed.
// It is possible for multiple watchers to watch the same session and
// GUID.
func (self *EventTraceWatcherService) Register(
	ctx context.Context,
	scope vfilter.Scope,
	session_name string, options ETWOptions,
	wGuid windows.GUID) (closer func(), output_chan chan vfilter.Row, err error) {
	session, err := self.SessionContext(session_name, scope, options)
	if err != nil {
		return nil, nil, err
	}

	return session.Register(ctx, scope, options, wGuid)
}

func (self *EventTraceWatcherService) SessionContext(
	name string, scope vfilter.Scope, options ETWOptions) (
	*SessionContext, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	sessionContext, pres := self.sessions[name]
	if !pres {
		// Create a new session
		sessionContext = NewSessionContext(name, options)
		self.sessions[name] = sessionContext
	}

	return sessionContext, nil
}

func (self *EventTraceWatcherService) Stats() []ProviderStat {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []ProviderStat{}
	for session_name, session_ctx := range self.sessions {
		for _, s := range session_ctx.Stats() {
			s.SessionName = session_name
			result = append(result, s)
		}
	}
	return result
}

func writeMetrics(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row) {
	for _, s := range GlobalEventTraceService.Stats() {
		output_chan <- ordereddict.NewDict().
			Set("SessionName", s.SessionName).
			Set("GUID", s.GUID).
			Set("Description", s.Description).
			Set("Watchers", s.Watchers).
			Set("EventCount", s.EventCount).
			Set("Started", s.Started).
			Set("Stats", s.Stats)
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "ETW",
		Description:   "Report the current state of the ETW subsystem",
		ProfileWriter: writeMetrics,
		Categories:    []string{"Global", "VQL", "Plugins"},
	})
}
