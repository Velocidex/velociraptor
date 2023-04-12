/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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

/*
Plugin Splunk.
*/
package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/clayscode/Go-Splunk-HTTP/splunk/v2"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _SplunkPluginArgs struct {
	Query          vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for rows to upload."`
	Threads        int64               `vfilter:"optional,field=threads,doc=How many threads to use."`
	URL            string              `vfilter:"optional,field=url,doc=The Splunk Event Collector URL."`
	Token          string              `vfilter:"optional,field=token,doc=Splunk HEC Token."`
	Index          string              `vfilter:"required,field=index,doc=The name of the index to upload to."`
	Source         string              `vfilter:"optional,field=source,doc=The source field for splunk. If not specified this will be 'velociraptor'."`
	Sourcetype     string              `vfilter:"optional,field=sourcetype,doc=The sourcetype field for splunk. If not specified this will 'vql'"`
	ChunkSize      int64               `vfilter:"optional,field=chunk_size,doc=The number of rows to send at the time."`
	SkipVerify     bool                `vfilter:"optional,field=skip_verify,doc=Skip SSL verification(default: False)."`
	RootCerts      string              `vfilter:"optional,field=root_ca,doc=As a better alternative to skip_verify, allows root ca certs to be added here."`
	WaitTime       int64               `vfilter:"optional,field=wait_time,doc=Batch splunk upload this long (2 sec)."`
	Hostname       string              `vfilter:"optional,field=hostname,doc=Hostname for Splunk Events. Defaults to server hostname."`
	TimestampField string              `vfilter:"optional,field=timestamp_field,doc=Field to use as event timestamp."`
	HostnameField  string              `vfilter:"optional,field=hostname_field,doc=Field to use as event hostname. Overrides hostname parameter."`
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

		config_obj, _ := artifacts.GetConfig(scope)

		wg := sync.WaitGroup{}
		row_chan := arg.Query.Eval(ctx, scope)
		for i := 0; i < int(arg.Threads); i++ {
			wg.Add(1)

			// Start an uploader on a thread.
			go _upload_rows(ctx, scope, config_obj, output_chan,
				row_chan, &wg, &arg)
		}

		wg.Wait()
	}()
	return output_chan
}

// Copy rows from row_chan to a local buffer and push it up to splunk.
func _upload_rows(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.ClientConfig,
	output_chan chan vfilter.Row,
	row_chan <-chan vfilter.Row,
	wg *sync.WaitGroup,
	arg *_SplunkPluginArgs) {
	defer wg.Done()

	// var buf []*ordereddict.Dict
	var buf = make([]vfilter.Row, 0, arg.ChunkSize)

	tlsConfig, err := networking.GetTlsConfig(config_obj, arg.RootCerts)
	if err != nil {
		scope.Log("splunk: cannot get TLS config: %s", err)
		return
	}

	if arg.SkipVerify {
		if err = networking.EnableSkipVerify(tlsConfig, config_obj); err != nil {
			scope.Log("splunk: cannot disable SSL security: %s", err)
			return
		}
	}

	client := splunk.NewClient(
		&http.Client{
			Timeout: time.Second * 20,
			Transport: &http.Transport{
				Proxy:           networking.GetProxy(),
				TLSClientConfig: tlsConfig,
			},
		}, // Optional HTTP Client objects
		arg.URL,
		arg.Token,
		arg.Source,
		arg.Sourcetype,
		arg.Index,
		arg.Hostname,
	)

	wait_time := time.Duration(arg.WaitTime) * time.Second
	next_send_time := time.After(wait_time)

	// Batch sending to splunk: Either when we get to chuncksize or
	// wait time whichever comes first.
	for {
		select {
		case row, ok := <-row_chan:
			if !ok {
				// Flush any remaining rows
				send_to_splunk(ctx, scope, output_chan, client, buf, arg)
				return
			}
			buf = append(buf, row)

			// Do not allow the buffer to get too large.
			if int64(len(buf)) > arg.ChunkSize {
				send_to_splunk(ctx, scope, output_chan, client, buf, arg)
				buf = buf[:0]
			}

		case <-next_send_time:
			send_to_splunk(ctx, scope, output_chan, client, buf, arg)
			buf = buf[:0]
			next_send_time = time.After(wait_time)
		}
	}
}

func send_to_splunk(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	client *splunk.Client, buf []vfilter.Row, arg *_SplunkPluginArgs) {

	if len(buf) == 0 {
		return
	}

	var events []*splunk.Event

	for _, event := range buf {
		// Extract hostname_field if exists
		var hostname string
		var ok bool
		dict := vfilter.RowToDict(ctx, scope, event)
		if arg.HostnameField != "" {
			hostname, ok = dict.GetString(arg.HostnameField)
			if !ok {
				scope.Log("ERROR:splunk_upload: %s not found, or isn't a string!", arg.HostnameField)
				return
			}
		}

		// Extract timestamp_field if exists
		if arg.TimestampField != "" {
			ts, ok := dict.Get(arg.TimestampField)
			if ok {
				timestamp, ok := functions.TimeFromAny(scope, ts)
				if ok != nil {
					// Default to start of Epoch if parse error
					timestamp = time.Date(1970, time.Month(1), 1, 0, 0, 0, 0, time.UTC)
				}
				events = append(
					events,
					client.NewEventWithTime(
						timestamp,
						dict,
						arg.Source,
						arg.Sourcetype,
						arg.Index,
						hostname,
					),
				)
			} else {
				scope.Log("ERROR:splunk_upload: %s not found!", arg.TimestampField)
			}
		} else {
			events = append(
				events,
				client.NewEvent(
					dict,
					arg.Source,
					arg.Sourcetype,
					arg.Index,
					hostname,
				),
			)
		}
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
			Set("Response", len(buf)):
		}
	}
}

func (self _SplunkPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "splunk_upload",
		Doc:      "Upload rows to splunk.",
		ArgType:  type_map.AddType(scope, &_SplunkPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_SplunkPlugin{})
}
