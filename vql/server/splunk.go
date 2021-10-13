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

/* Plugin Splunk.


 */
package server

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/ZachtimusPrime/Go-Splunk-HTTP/splunk/v2"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _SplunkPluginArgs struct {
	Query      vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for rows to upload."`
	Threads    int64               `vfilter:"optional,field=threads,doc=How many threads to use."`
	URL        string              `vfilter:"optional,field=url,doc=The Splunk Event Collector URL."`
	Token      string              `vfilter:"optional,field=token,doc=Splunk HEC Token."`
	Index      string              `vfilter:"required,field=index,doc=The name of the index to upload to."`
	Source     string              `vfilter:"optional,field=source,doc=The source field for splunk. If not specified this will be 'velociraptor'."`
	Sourcetype string              `vfilter:"optional,field=sourcetype,doc=The sourcetype field for splunk. If not specified this will 'vql'"`
	ChunkSize  int64               `vfilter:"optional,field=chunk_size,doc=The number of rows to send at the time."`
	SkipVerify bool                `vfilter:"optional,field=skip_verify,doc=Skip SSL verification(default: False)."`

	WaitTime int64 `vfilter:"optional,field=wait_time,doc=Batch splunk upload this long (2 sec)."`
}

type _SplunkPlugin struct{}

func (self _SplunkPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
		if err != nil {
			return
		}

		arg := _SplunkPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, &arg)
		if err != nil {
			return
		}

		if arg.Threads == 0 {
			arg.Threads = 1
		}

		if arg.ChunkSize == 0 {
			arg.ChunkSize = 1000
		}

		if arg.WaitTime == 0 {
			arg.WaitTime = 2
		}

		if len(arg.Sourcetype) == 0 {
			arg.Sourcetype = "vql"
		}

		if len(arg.Source) == 0 {
			arg.Source = "velociraptor"
		}

		wg := sync.WaitGroup{}
		row_chan := arg.Query.Eval(ctx, scope)
		for i := 0; i < int(arg.Threads); i++ {
			wg.Add(1)

			// Start an uploader on a thread.
			go _upload_rows(ctx, scope, output_chan,
				row_chan, &wg, &arg)
		}

		wg.Wait()
	}()
	return output_chan
}

// Copy rows from row_chan to a local buffer and push it up to splunk.
func _upload_rows(
	ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row,
	row_chan <-chan vfilter.Row,
	wg *sync.WaitGroup,
	arg *_SplunkPluginArgs) {
	defer wg.Done()

	// var buf []*ordereddict.Dict
	var buf = make([]vfilter.Row, 0, arg.ChunkSize)

	client := splunk.NewClient(
		&http.Client{
			Timeout: time.Second * 20,
			Transport: &http.Transport{
				Proxy:           networking.GetProxy(),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: arg.SkipVerify},
			},
		}, // Optional HTTP Client objects
		arg.URL,
		arg.Token,
		arg.Source,
		arg.Sourcetype,
		arg.Index,
	)

	wait_time := time.Duration(arg.WaitTime) * time.Second
	next_send_time := time.After(wait_time)

	// Flush any remaining rows
	defer send_to_splunk(ctx, scope, output_chan, client, &buf, arg)

	// Batch sending to splunk: Either
	// when we get to chuncksize or wait
	// time whichever comes first.
	for {
		select {
		case row, ok := <-row_chan:
			if !ok {
				return
			}

			//
			buf = append(buf, row)

		case <-next_send_time:

			send_to_splunk(ctx, scope, output_chan,
				client, &buf, arg)

			next_send_time = time.After(wait_time)
		}
	}
}

func send_to_splunk(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	client *splunk.Client, buf *[]vfilter.Row, arg *_SplunkPluginArgs) {

	_buf := *buf

	if len(_buf) == 0 {
		return
	}

	var events []*splunk.Event

	for _, event := range _buf {
		events = append(
			events,
			client.NewEvent(
				event,
				arg.Source,
				arg.Sourcetype,
				arg.Index,
			),
		)
	}

	err := client.LogEvents(events)

	if err != nil {
		select {
		case <-ctx.Done():
			return
		case output_chan <- ordereddict.NewDict().
			Set("Response", err):
		}
	} else {
		select {
		case <-ctx.Done():
			return
		case output_chan <- ordereddict.NewDict().
			Set("Response", len(_buf)):
		}
	}

	// clear the slice
	*buf = _buf[:0]

}

func (self _SplunkPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "splunk_upload",
		Doc:     "Upload rows to splunk.",
		ArgType: type_map.AddType(scope, &_SplunkPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_SplunkPlugin{})
}
