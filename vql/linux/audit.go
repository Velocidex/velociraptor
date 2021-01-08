// +build linux

package linux

import (
	"context"
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/elastic/go-libaudit"
	"github.com/elastic/go-libaudit/aucoalesce"
	"github.com/elastic/go-libaudit/auparse"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type streamHandler struct {
	scope       vfilter.Scope
	output_chan chan vfilter.Row
}

func (self *streamHandler) ReassemblyComplete(msgs []*auparse.AuditMessage) {
	self.outputMultipleMessages(msgs)
}

func (self *streamHandler) EventsLost(count int) {
	self.scope.Log("Detected the loss of %v sequences.", count)
}

func (self *streamHandler) outputMultipleMessages(msgs []*auparse.AuditMessage) {
	event, err := aucoalesce.CoalesceMessages(msgs)
	if err != nil {
		return
	}
	self.output_chan <- event
}

type AuditPlugin struct{}

func (self AuditPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "audit",
		Doc:  "Register as an audit daemon in the kernel.",
	}
}

func (self AuditPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("audit: %s", err)
			return
		}

		client, err := libaudit.NewMulticastAuditClient(nil)
		if err != nil {
			scope.Log("audit: %v", err)
			return
		}
		defer client.Close()

		reassembler, err := libaudit.NewReassembler(5, 2*time.Second,
			&streamHandler{scope, output_chan})
		if err != nil {
			scope.Log("audit: %v", err)
			return
		}
		defer reassembler.Close()

		// Start goroutine to periodically purge timed-out events.
		go func() {
			t := time.NewTicker(500 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return

				case <-t.C:
					if reassembler.Maintain() != nil {
						return
					}
				}
			}
		}()

		for {
			rawEvent, err := client.Receive(false)
			if err != nil {
				scope.Log("receive failed: %s", err)
				continue
			}

			// Messages from 1300-2999 are valid audit messages.
			if rawEvent.Type < auparse.AUDIT_USER_AUTH ||
				rawEvent.Type > auparse.AUDIT_LAST_USER_MSG2 {
				continue
			}

			line := fmt.Sprintf("type=%v msg=%v\n", rawEvent.Type, string(rawEvent.Data))
			auditMsg, err := auparse.ParseLogLine(line)
			if err == nil {
				reassembler.PushMessage(auditMsg)
			}
		}
	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&AuditPlugin{})
}
