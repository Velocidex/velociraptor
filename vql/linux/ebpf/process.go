package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type velo_proc_event ebpf process.c

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"

	"github.com/Velocidex/ordereddict"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	kprobeFunc = "sys_execve"
)

type ProcessEventPlugin struct{}

func (self ProcessEventPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "ebpf_process",
		Doc:      "Read process execution events from ebpf.",
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func (self ProcessEventPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor("ebpf_process", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("ebpf_process: %s", err)
			return
		}

		// Allow the current process to lock memory for eBPF resources.
		err = rlimit.RemoveMemlock()
		if err != nil {
			scope.Log("ebpf_process: %s", err)
			return
		}

		objs := ebpfObjects{}
		err = loadEbpfObjects(&objs, nil)
		if err != nil {
			scope.Log("ebpf_process: %s", err)

			var verr *ebpf.VerifierError
			if errors.As(err, &verr) {
				scope.Log("%+v\n", verr)
			}
			return
		}
		defer objs.Close()

		kp, err := link.Kprobe(kprobeFunc, objs.HookExecve, nil)
		if err != nil {
			scope.Log("ebpf_process: %s", err)
			return
		}
		defer kp.Close()

		rd, err := ringbuf.NewReader(objs.Events)
		if err != nil {
			scope.Log("ebpf_process: %s", err)
			return
		}
		defer rd.Close()

		go func() {
			<-ctx.Done()

			err := rd.Close()
			if err != nil {
				scope.Log("ebpf_process: %s", err)
				return
			}
		}()

		var event ebpfVeloProcEvent
		for {
			record, err := rd.Read()
			if err != nil {
				if errors.Is(err, ringbuf.ErrClosed) {
					return
				}
				scope.Log("ebpf_process: reading from reader: %v", err)
				continue
			}

			err = binary.Read(bytes.NewBuffer(record.RawSample),
				binary.LittleEndian, &event)
			if err != nil {
				scope.Log("ebpf_process: parsing ringbuf event: %s", err)
				continue
			}

			row := ordereddict.NewDict().
				Set("timestamp", event.Ktime).
				Set("pid", event.Pid).
				Set("ppid", event.Ppid).
				Set("parent_comm", unix.ByteSliceToString(event.ParentComm[:])).
				Set("exe", unix.ByteSliceToString(event.Exe[:])).
				Set("exe_path", unix.ByteSliceToString(event.ExePath[:])).
				Set("comm", unix.ByteSliceToString(event.Comm[:]))

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}

	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&ProcessEventPlugin{})
}
