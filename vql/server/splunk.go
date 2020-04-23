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
	"github.com/ZachtimusPrime/Go-Splunk-HTTP/splunk"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
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
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	scope.Log("set up splunk")

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
		if err != nil {
			return
		}

		arg := _SplunkPluginArgs{}
		err = vfilter.ExtractArgs(scope, args, &arg)
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
	scope *vfilter.Scope, output_chan chan vfilter.Row,
	row_chan <-chan vfilter.Row,
	wg *sync.WaitGroup,
	arg *_SplunkPluginArgs) {
	defer wg.Done()

	var buf []*ordereddict.Dict

	client := splunk.NewClient(
		&http.Client{
			Timeout: time.Second * 20,
			Transport: &http.Transport{
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
	defer send_to_splunk(scope, output_chan, client, &buf, arg)

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
			err := _append_row_to_buffer(ctx, scope, row, &buf, arg)
			if err != nil {
				continue
			}

		case <-next_send_time:

			send_to_splunk(scope, output_chan,
				client, &buf, arg)

			next_send_time = time.After(wait_time)
		}
	}
}

func _append_row_to_buffer(
	ctx context.Context,
	scope *vfilter.Scope,
	row vfilter.Row, buf *[]*ordereddict.Dict,
	arg *_SplunkPluginArgs) error {

	_buf := *buf
	row_dict := vfilter.RowToDict(ctx, scope, row)

	// if ClientID exists and "Host" isn't a field, use the ClientID field
	clientid, client_idpres := row_dict.Get("ClientId")
	_, host_pres := row_dict.Get("host")

	if !host_pres {
		if client_idpres {
			row_dict = row_dict.Set("host", clientid)
		} else {
			row_dict = row_dict.Set("host", "velociraptor")
		}
	}
	*buf = append(_buf, row_dict)
	return nil
}

func send_to_splunk(scope *vfilter.Scope,
	output_chan chan vfilter.Row,
	client *splunk.Client, buf *[]*ordereddict.Dict, arg *_SplunkPluginArgs) {

	_buf := *buf

	scope.Log("buf: %d", len(_buf))

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
		output_chan <- ordereddict.NewDict().
			Set("Response", err)
	} else {
		output_chan <- ordereddict.NewDict().
			Set("Response", len(_buf))
	}

	// clear the slice
	*buf = _buf[:0]

}

func (self _SplunkPlugin) Info(
	scope *vfilter.Scope,
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
