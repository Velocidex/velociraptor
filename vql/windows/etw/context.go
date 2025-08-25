//go:build windows && cgo && amd64
// +build windows,cgo,amd64

package etw

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/etw"
	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type Registration struct {
	handles     []*Handle
	count       int
	description string
	started     time.Time
}

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
	registrations map[string]*Registration

	resolve_map_info bool

	// Options used for the kernel tracer provider. These apply for
	// the entire session.
	rundown_options etw.RundownOptions

	// Set to true once the session is already processing
	is_processing uint64

	is_kernel_trace bool

	kernel_info_manager *etw.KernelInfoManager

	process_metadata *ttlcache.Cache
}

func NewSessionContext(name string, options ETWOptions) *SessionContext {
	res := &SessionContext{
		name:             name,
		registrations:    make(map[string]*Registration),
		rundown_options:  options.RundownOptions,
		process_metadata: ttlcache.NewCache(),
	}

	_ = res.process_metadata.SetTTL(time.Minute)
	res.process_metadata.SetCacheSizeLimit(1000)
	return res
}

func (self *SessionContext) Close() {
	self.process_metadata.Close()
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

	options ETWOptions
}

func (self *Handle) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.closed = true

	self.cancel()
	close(self.output_chan)
}

// Try to send but skip closed handles.
func (self *Handle) Send(event *etw.Event) {
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

// The Kernel Trace provider is a bit different:

//  1. We do not subscribe to the trace provider - the library does
//     this automatically.
//  2. Session name has to be etw.KernelTraceSessionName - not user
//     selectable.
//  3. Only one provider is available and we do not need to subscribe
//     to it.
//  4. The events returned on this session belong to many internal
//     providers and are not related to the specific listener
//     registered on this session. Therefore for kernel sessions all
//     registrations will see all events.
func (self *SessionContext) _KernelTraceSession(
	scope vfilter.Scope) (*etw.Session, error) {

	session, err := etw.NewKernelTraceSession(
		self.rundown_options, self.processEvent)
	if err != nil {
		err = etw.KillSession(etw.KernelTraceSessionName)
		if err != nil {
			return nil, err
		}

		session, err = etw.NewKernelTraceSession(
			self.rundown_options, self.processEvent)
		if err != nil {
			return nil, err
		}
	}

	self.is_kernel_trace = true

	scope.Log("etw: Started kernel session with options %v",
		optionsString(self.rundown_options))
	return session, nil
}

func (self *SessionContext) _CreateTraceSession(
	scope vfilter.Scope) (*etw.Session, error) {
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

	return session, nil
}

func (self *SessionContext) _Session(
	scope vfilter.Scope) (*etw.Session, error) {

	var err error

	// Cache the session for reuse.
	if self.session != nil {
		return self.session, nil
	}

	// If the user asked for the kernel trace session we need to
	// create a special session here.
	if strings.EqualFold(self.name, etw.KernelTraceSessionName) {
		self.session, err = self._KernelTraceSession(scope)
	} else {
		self.session, err = self._CreateTraceSession(scope)
	}
	if err != nil {
		return nil, err
	}

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()

		err := self.session.Process()
		if err != nil {
			scope.Log("etw: Can not start session %v: %v", self.name, err)
		}
	}()

	return self.session, nil
}

func (self *SessionContext) processEvent(e *etw.Event) {
	defer utils.CheckForPanic("processEvent")

	e.Props()

	event_guid := e.Header.ProviderID.String()
	var handlers []*Handle

	self.mu.Lock()

	// For the kernel trace all handlers will be called with every
	// event.
	is_kernel_trace := self.is_kernel_trace
	if is_kernel_trace {
		for _, v := range self.registrations {
			handlers = append(handlers, v.handles...)
			v.count++
		}

	} else {
		registration, pres := self.registrations[event_guid]
		if !pres {
			self.mu.Unlock()
			return
		}

		handlers = append(handlers, registration.handles...)
		registration.count++
	}

	self.mu.Unlock()

	self.enrichEvent(event_guid, e)

	// Send the event to all interested parties.
	for _, handle := range handlers {
		// The Kernel Trace Provider emits events from many internal
		// providers, even if we did not subscribe to them
		// specifically.
		if is_kernel_trace ||
			handle.guid == e.Header.ProviderID {
			handle.Send(e)
		}
	}
}

func (self *SessionContext) kernelInfoManager() *etw.KernelInfoManager {
	if self.kernel_info_manager == nil {
		self.kernel_info_manager = etw.NewKernelInfoManager()
	}

	return self.kernel_info_manager
}

func fixInt(props *ordereddict.Dict, field string) {
	value_str, ok := props.GetString(field)
	if ok {
		value, err := strconv.ParseInt(value_str, 0, 64)
		if err == nil {
			props.Update(field, value)
		}
	}
}

func (self *SessionContext) fixFilename(
	props *ordereddict.Dict, field string) {
	filename, ok := props.GetString(field)
	if ok {
		props.Update(field, self.kernelInfoManager().
			NormalizeFilename(filename))
	}
}

func (self *SessionContext) getProcessMetadata(
	props *ordereddict.Dict) *ordereddict.Dict {
	pid, pres := props.GetInt64("ProcessId")
	if pres {
		PidStr := strconv.FormatInt(pid, 10)
		process_info, err := self.process_metadata.Get(PidStr)
		if err != nil || process_info == nil {
			process_info = ordereddict.NewDict()
			self.process_metadata.Set(PidStr, process_info)
		}
		return process_info.(*ordereddict.Dict)
	}
	return nil
}

func (self *SessionContext) enrichEvent(guid string, e *etw.Event) {
	props := e.Props()
	fixInt(props, "ProcessId")
	fixInt(props, "ParentId")

	// Try to resolve filenames into normal paths from kernel paths.
	self.fixFilename(props, "FileName")

	// Special handling for various providers.

	// Currently no special handling.
	/*
		switch guid {
		// Microsoft-Windows-Kernel-File
		// We resolve the paths into filesystem names instead of kernel paths.
		case "{EDD08927-9CC4-4E65-B970-C2560FB5C289}":

			// "KernelEventType":"LoadImage"
		case "{2CB15D1D-5FC1-11D2-ABE1-00A0C911F518}":

		case "{3D6FA8D0-FE05-11D0-9DDA-00C04FD7BA7C}":

			// Microsoft-Windows-Kernel-Process
		case "{22FB2CD6-0E7B-422B-A0C7-2FAD1FD0E716}":
		}
	*/
}

func (self *SessionContext) Stats() []ProviderStat {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []ProviderStat{}
	for guid, registration := range self.registrations {
		res := ProviderStat{
			GUID:        guid,
			EventCount:  registration.count,
			Description: registration.description,
			Watchers:    len(registration.handles),
			Started:     registration.started,
		}

		if self.is_kernel_trace {
			res.Stats = etw.KernelInfo.Stats()
			res.Stats.Set("Options", optionsString(self.rundown_options))
		}
		result = append(result, res)
	}

	return result
}

// Check the new options to see if they are compatible with the
// existing session. If not we need to update the session to cover
// the comabined options for old watchers and new watchers.
func (self *SessionContext) ensureOptionsValid(
	scope vfilter.Scope, registration *Registration) (err error) {
	if self.is_kernel_trace {
		new_rundown := &etw.RundownOptions{}
		// Merge all options from all handles
		for _, h := range registration.handles {
			megrgeRundown(new_rundown, h.options.RundownOptions)
		}

		// If the new options are compatible with the old options we
		// need to restart the session.
		if *new_rundown != self.rundown_options {
			self.rundown_options = *new_rundown

			// Update the session options
			scope.Log("etw: Reconfiguring ETW session for new options: %v",
				optionsString(self.rundown_options))

			return etw.UpdateKernelTraceOptions(self.session, self.rundown_options)
		}
	}

	return err
}

// Callers call this to register a watcher on the GUID
func (self *SessionContext) Register(
	ctx context.Context,
	scope vfilter.Scope, options ETWOptions,
	guid windows.GUID) (closer func(), output_chan chan vfilter.Row, err error) {

	key := guid.String()
	handle := NewHandle(ctx, scope, guid)
	handle.options = options

	self.mu.Lock()
	defer self.mu.Unlock()

	if options.EnableMapInfo {
		self.resolve_map_info = options.EnableMapInfo
	}

	registration, pres := self.registrations[key]
	if pres {
		// Add the handle to the old session
		registration.handles = append(registration.handles, handle)
		self.registrations[key] = registration

		// Create a new session
	} else {
		registration = &Registration{
			description: options.Description,
			started:     utils.GetTime().Now(),
			handles:     []*Handle{handle},
		}

		self.registrations[key] = registration

		// No one is currently watching this GUID, lets begin watching
		// it.
		session, err := self._Session(scope)
		if err != nil {
			return nil, nil, err
		}

		// The kernel trace sessions can only contain a single
		// provider and we do not need to subscribe to it.
		if !self.is_kernel_trace {
			opts := etw.SessionOptions{
				Name:            self.name,
				Guid:            guid,
				Level:           etw.TraceLevel(options.Level),
				MatchAnyKeyword: options.AnyKeyword,
				MatchAllKeyword: options.AllKeyword,
				CaptureState:    options.CaptureState,
			}
			err = session.SubscribeToProvider(opts)
			if err != nil {
				scope.Log("etw: Can not add provider to session %v: %v", self.name, err)
				return nil, nil, err
			}
			scope.Log("etw: Added provider %v to session %v", guid.String(), self.name)
		}
	}

	err = self.ensureOptionsValid(scope, registration)
	if err != nil {
		return nil, nil, err
	}

	return func() {
		self.DeregisterHandle(key, handle.id, guid, scope)
	}, handle.output_chan, nil
}

// Remove the handle from the set of registrations
func (self *SessionContext) DeregisterHandle(
	key string, id uint64, guid windows.GUID, scope vfilter.Scope) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registration, pres := self.registrations[key]
	if !pres {
		// No registrations for this provider
		return
	}

	// Remove the handle from the registrations
	new_reg := make([]*Handle, 0, len(registration.handles))
	for _, handle := range registration.handles {
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
		registration.handles = new_reg
		self.ensureOptionsValid(scope, registration)
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
