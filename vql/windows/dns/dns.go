// +build windows

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package dns

// #cgo LDFLAGS: -lws2_32
//
// void *watchDNS(void *go_ctx);
// void runDNS(void *c_ctx);
// void destroyDNS(void *c_ctx);
import "C"
import (
	"context"
	"fmt"
	"time"
	"unsafe"

	pointer "github.com/mattn/go-pointer"
	"golang.org/x/net/dns/dnsmessage"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// We do not want to be too general here since it will make the VQL
// more complex. Most of the time we only care about A and AAAA
// records possible with a CNAME returned. We aim to build a single
// DNSEvent per packet and return that.
type DNSEvent struct {
	// Q for question, A for Answer.
	EventType string

	Time int64

	// The name the resource is about.
	Name string

	// Any CNAMEs returned.
	CNAME []string

	// Encoded IP addresses - both ip4 and ip6 go here.
	Answers []string

	sent bool
}

func inet_ntoa(ip [4]byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", ip[0],
		ip[1], ip[2], ip[3])
}

type eventContext struct {
	output chan vfilter.Row
	scope  *vfilter.Scope
}

func (self *eventContext) ProcessEvent(packet []byte) {
	parser := dnsmessage.Parser{}
	header, err := parser.Start(packet)
	if err != nil {
		return
	}

	send_no_block := func(event *DNSEvent) {
		select {
		case self.output <- event:
		default:
		}
	}

	// This is a request, we just need to return it.
	if !header.Response {
		questions, err := parser.AllQuestions()
		if err != nil {
			return
		}

		for _, question := range questions {
			send_no_block(&DNSEvent{
				EventType: "Q",
				Time:      time.Now().Unix(),
				Name:      question.Name.String(),
			})
		}
	} else {
		// If it is a response we just want the answers.
		err := parser.SkipAllQuestions()
		if err != nil {
			return
		}

		all_answers, err := parser.AllAnswers()
		if err != nil {
			return
		}
		// We want to group together answers for each question
		// so we can send them as a single event.
		lookup := make(map[string]*DNSEvent)
		for _, answer := range all_answers {
			name := answer.Header.Name.String()
			event, pres := lookup[name]
			if !pres {
				event = &DNSEvent{
					EventType: "A",
					Time:      time.Now().Unix(),
					Name:      name,
				}
				lookup[name] = event
			}

			switch t := answer.Body.(type) {
			case *dnsmessage.AResource:
				event.Answers = append(
					event.Answers, inet_ntoa(t.A))

			case *dnsmessage.CNAMEResource:
				cname := t.CNAME.String()
				event.CNAME = append(event.CNAME, cname)
				lookup[cname] = event

			default:
				continue
			}
		}

		for _, event := range lookup {
			if !event.sent {
				send_no_block(event)
			}
			event.sent = true
		}
	}
}

//export process_dns
func process_dns(ctx *C.int, buff *C.char, length C.int) {
	if ctx == nil {
		return
	}

	restored_ctx := pointer.Restore(unsafe.Pointer(ctx))
	if restored_ctx != nil {
		go_ctx := restored_ctx.(*eventContext)
		go_buff := (*[1 << 30]byte)(unsafe.Pointer(buff))[:length]
		go_ctx.ProcessEvent(go_buff)
	}
}

type DNSEventPluginArgs struct{}

type DNSEventPlugin struct{}

func (self DNSEventPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &DNSEventPluginArgs{}

	go func() {
		defer close(output_chan)

		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("dns: %s", err.Error())
			return
		}

		event_context := eventContext{
			output: make(chan vfilter.Row, 100),
			scope:  scope,
		}

		ptr := pointer.Save(&event_context)
		defer pointer.Unref(ptr)

		c_ctx := C.watchDNS(ptr)
		if c_ctx == nil {
			return
		}

		// When the scope is destroyed we want to quit reading
		// from the DNS generator immediately.
		sub_ctx, cancel := context.WithCancel(ctx)
		scope.AddDestructor(func() {
			C.destroyDNS(c_ctx)
			cancel()
		})

		// Run concurrently.
		go func() {
			C.runDNS(c_ctx)

			// When we return from runDNS we know it is
			// impossible to write on this channel any
			// more so we can close it here.
			defer close(event_context.output)
		}()

		for {
			select {
			case <-sub_ctx.Done():
				return

				// Read the next item from the event
				// queue and send it to the VQL
				// subsystem.
			case item, ok := <-event_context.output:
				if !ok {
					return
				}
				output_chan <- item
			}
		}
	}()

	return output_chan
}

func (self DNSEventPlugin) Info(
	scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "dns",
		Doc:     "Monitor dns queries.",
		ArgType: type_map.AddType(scope, &DNSEventPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DNSEventPlugin{})
}
