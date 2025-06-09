//go:build windows && amd64
// +build windows,amd64

package etw

import (
	"context"
	"strings"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WNODE_HEADER struct {
	BufferSize        uint32
	ProviderId        uint32
	HistoricalContext uint64
	KernelHandle      windows.HANDLE // union TimeStamp
	GUID              windows.GUID
	ClientContext     uint32
	Flags             uint32
}

type EVENT_TRACE_PROPERTIES struct {
	Wnode               WNODE_HEADER
	BufferSize          uint32
	MinimumBuffers      uint32
	MaximumBuffers      uint32
	MaximumFileSize     uint32
	LogFileMode         uint32
	FlushTimer          uint32
	EnableFlags         uint32
	AgeLimit            uint32
	NumberOfBuffers     uint32
	FreeBuffers         uint32
	EventsLost          uint32
	BuffersWritten      uint32
	LogBuffersLost      uint32
	RealTimeBuffersLost uint32
	LoggerThreadId      windows.HANDLE
	LogFileNameOffset   uint32
	LoggerNameOffset    uint32
}

type EVENT_TRACE_PROPERTIES_WithNames struct {
	EVENT_TRACE_PROPERTIES

	SessionName [1024]byte
	LogfilePath [1024]byte
}

func nullTerminated(buf [1024]byte) string {
	return strings.Split(string(buf[:]), "\x00")[0]
}

type EtwSessionsArgs struct {
	SessionCount uint64 `vfilter:"optional,field=count,doc=The count of sessions to retrieve (default 64) "`
}

type EtwSessions struct{}

func (self EtwSessions) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "etw_sessions", args)()

		arg := &EtwSessionsArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("etw_sessions: %s", err.Error())
			return
		}

		if arg.SessionCount == 0 {
			arg.SessionCount = 64
		}

		if arg.SessionCount > 1000 {
			arg.SessionCount = 1000
		}

		err = vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			return
		}

		sessions := make([]*EVENT_TRACE_PROPERTIES_WithNames, 0, arg.SessionCount)
		for i := uint64(0); i < arg.SessionCount; i++ {
			var session EVENT_TRACE_PROPERTIES_WithNames
			session.Wnode.BufferSize = uint32(unsafe.Sizeof(session))
			session.LoggerNameOffset = uint32(unsafe.Sizeof(session.EVENT_TRACE_PROPERTIES))
			session.LogFileNameOffset = uint32(unsafe.Sizeof(session.EVENT_TRACE_PROPERTIES) +
				unsafe.Sizeof(session.SessionName))

			sessions = append(sessions, &session)
		}

		session_count := uint32(arg.SessionCount)
		status := windows.QueryAllTracesW(
			uintptr(unsafe.Pointer(&sessions[0])),
			session_count, &session_count)
		if status != windows.STATUS_SUCCESS {
			scope.Log("etw_sessions: QueryAllTracesW returned %v (%v)",
				windows.NTStatus_String(status), status)
			return
		}

		for i := uint32(0); i < session_count; i++ {
			session := sessions[i]
			row := ordereddict.NewDict().
				Set("SessionGUID", session.Wnode.GUID.String()).
				Set("SessionID", session.Wnode.HistoricalContext).
				Set("SessionName", nullTerminated(session.SessionName)).
				Set("LogFileName", nullTerminated(session.LogfilePath)).
				Set("MinBuffers", session.MinimumBuffers).
				Set("MaxBuffers", session.MaximumBuffers).
				Set("NumberOfBuffers", session.NumberOfBuffers).
				Set("BuffersWritten", session.BuffersWritten).
				Set("BuffersLost", session.LogBuffersLost).
				Set("EventsLost", session.EventsLost)

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}

	}()

	return output_chan
}

func (self EtwSessions) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "etw_sessions",
		Doc:      "Enumerates all active ETW sessions",
		ArgType:  type_map.AddType(scope, &EtwSessionsArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&EtwSessions{})
}
