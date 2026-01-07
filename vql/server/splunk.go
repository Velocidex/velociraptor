/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/clayscode/Go-Splunk-HTTP/splunk/v2"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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
	Index          string              `vfilter:"optional,field=index,doc=The name of the index to upload to. If not specified, ensure a column is named _splunk_index."`
	Source         string              `vfilter:"optional,field=source,doc=The source field for splunk. If not specified ensure a column is named _splunk_source or this will be 'velociraptor'."`
	SourceType     string              `vfilter:"optional,field=sourcetype,doc=The sourcetype field for splunk. If not specified ensure a column is named _splunk_source_type or this will 'vql'"`
	ChunkSize      int64               `vfilter:"optional,field=chunk_size,doc=The number of rows to send at the time."`
	SkipVerify     bool                `vfilter:"optional,field=skip_verify,doc=Skip SSL verification(default: False)."`
	RootCerts      string              `vfilter:"optional,field=root_ca,doc=As a better alternative to skip_verify, allows root ca certs to be added here."`
	WaitTime       int64               `vfilter:"optional,field=wait_time,doc=Batch splunk upload this long (2 sec)."`
	Hostname       string              `vfilter:"optional,field=hostname,doc=Hostname for Splunk Events. Defaults to server hostname."`
	TimestampField string              `vfilter:"optional,field=timestamp_field,doc=Field to use as event timestamp."`
	HostnameField  string              `vfilter:"optional,field=hostname_field,doc=Field to use as event hostname. Overrides hostname parameter."`
	Secret         string              `vfilter:"optional,field=secret,doc=Alternatively use a secret from the secrets service. Secret must be of type 'Splunk'"`
	MaxRetries     int64               `vfilter:"optional,field=max_retries,doc=Maximum number of retries for failed uploads (default: 3)."`
	RetryWait      int64               `vfilter:"optional,field=retry_wait,doc=Base wait time in seconds for exponential backoff between retries (default: 2). Actual wait times: 2s, 4s, 8s, 16s..."`
	IdleConnTimeout int64              `vfilter:"optional,field=idle_conn_timeout,doc=How long to keep idle HTTP connections open in seconds (default: 55). Lower values help with firewalls/load balancer/HECs that close connections."`
}

type _SplunkPlugin struct{}

func (self _SplunkPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "splunk_upload", args)()

		err := vql_subsystem.CheckAccess(scope, acls.NETWORK)
		if err != nil {
			scope.Log("splunk_upload: %v", err)
			return
		}

		arg := &_SplunkPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("splunk_upload: %v", err)
			return
		}

		err = self.maybeForceSecrets(ctx, scope, arg)
		if err != nil {
			scope.Log("splunk_upload: %v", err)
			return
		}

		if arg.Secret != "" {
			err := mergeSecretSplunk(ctx, scope, arg)
			if err != nil {
				scope.Log("splunk_upload: %v", err)
				return
			}
		}

		if arg.URL == "" {
			scope.Log("splunk_upload: field url is required")
			return
		}

		if arg.Index == "" {
			scope.Log("splunk_upload: field index is required")
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

		if arg.MaxRetries == 0 {
			arg.MaxRetries = 3
		}

		if arg.RetryWait == 0 {
			arg.RetryWait = 2
		}

		if arg.IdleConnTimeout == 0 {
			arg.IdleConnTimeout = 55
		}

		if len(arg.SourceType) == 0 {
			arg.SourceType = "vql"
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
				row_chan, &wg, arg)
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
				IdleConnTimeout: time.Duration(arg.IdleConnTimeout) * time.Second,
			},
		}, // Optional HTTP Client objects
		arg.URL,
		arg.Token,
		arg.Source,
		arg.SourceType,
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

		source, ok := dict.GetString("_splunk_source")
		if !ok {
			source = arg.Source
		} else {
			dict.Delete("_splunk_source")
		}

		source_type, ok := dict.GetString("_splunk_source_type")
		if !ok {
			source_type = arg.SourceType
		} else {
			dict.Delete("_splunk_source_type")
		}

		index, ok := dict.GetString("_splunk_index")
		if !ok {
			index = arg.Index
		} else {
			dict.Delete("_splunk_index")
		}

		// Extract timestamp_field if exists
		if arg.TimestampField != "" {
			ts, ok := dict.Get(arg.TimestampField)
			if ok {
				timestamp, ok := functions.TimeFromAny(ctx, scope, ts)
				if ok != nil {
					// Default to start of Epoch if parse error
					timestamp = time.Date(1970, time.Month(1), 1, 0, 0, 0, 0, time.UTC)
				}
				events = append(
					events,
					client.NewEventWithTime(
						timestamp,
						dict,
						source,
						source_type,
						index,
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
					source,
					source_type,
					index,
					hostname,
				),
			)
		}
	}

	// Attempt to send events with retry logic
	var err error
	for attempt := int64(0); attempt <= arg.MaxRetries; attempt++ {
		err = client.LogEvents(events)

		if err == nil {
			// Success
			select {
			case <-ctx.Done():
				return
			case output_chan <- ordereddict.NewDict().
				Set("Response", len(buf)):
			}
			return
		}

		// If this wasn't the last attempt, wait before retrying with exponential backoff
		if attempt < arg.MaxRetries {
			// Exponential backoff: retry_wait * 2^attempt (e.g., 2s, 4s, 8s, 16s)
			backoffWait := time.Duration(arg.RetryWait*(1<<attempt)) * time.Second
			scope.Log("splunk_upload: attempt %d failed: %v, retrying in %v...", attempt+1, err, backoffWait)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoffWait):
				// Continue to next retry
			}
		}
	}

	// All retries exhausted
	scope.Log("ERROR:splunk_upload: all %d attempts failed: %v", arg.MaxRetries+1, err)
	select {
	case <-ctx.Done():
		return
	case output_chan <- ordereddict.NewDict().
		Set("Response", err.Error()):
	}
}

func (self _SplunkPlugin) maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope, arg *_SplunkPluginArgs) error {

	// Not running on the server, secrets dont work.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil
	}

	if config_obj.Security == nil {
		return nil
	}

	if !config_obj.Security.VqlMustUseSecrets {
		return nil
	}

	// If an explicit secret is defined let it filter the URLs.
	if arg.Secret != "" {
		return nil
	}

	return utils.SecretsEnforced
}

func mergeSecretSplunk(ctx context.Context, scope vfilter.Scope, arg *_SplunkPluginArgs) error {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return errors.New("splunk_upload: Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	s, err := secrets_service.GetSecret(ctx, principal,
		constants.SPLUNK_CREDS, arg.Secret)
	if err != nil {
		return err
	}

	// Allow the user to override these fields
	s.UpdateString("source", &arg.Source)
	s.UpdateString("index", &arg.Index)
	s.UpdateString("hostname_field", &arg.HostnameField)
	s.UpdateString("hostname", &arg.Hostname)

	arg.URL = s.GetString("url")
	arg.Token = s.GetString("token")
	arg.RootCerts = s.GetString("root_ca")
	arg.SkipVerify = s.GetBool("skip_verify")

	return nil
}

func (self _SplunkPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "splunk_upload",
		Doc:      "Upload rows to splunk.",
		ArgType:  type_map.AddType(scope, &_SplunkPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.NETWORK).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_SplunkPlugin{})
}
