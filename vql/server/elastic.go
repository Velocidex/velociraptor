// This module addes a dependency on go-elasticsearch which turns out to be
// huge!

// $ goweight ./bin/
//    15 MB github.com/elastic/go-elasticsearch/v7/esapi

// We observe a 6mb increase in the binary for this dependency which
// was deemed unacceptable. Further investigation revealed the size
// was because the API Surface is huge and the client library supports
// it all. Since we only actually bulk upload data to elastic we do
// not need the entire API anyway. We therefore maintain a fork of the
// client library for now. This allows us to include it in all builds
// with a very minimal footprint.

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
Plugin Elastic.
*/
package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	elasticsearch "github.com/Velocidex/go-elasticsearch/v7"
	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _ElasticPluginArgs struct {
	Query              vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for rows to upload."`
	Threads            int64               `vfilter:"optional,field=threads,doc=How many threads to use."`
	Index              string              `vfilter:"optional,field=index,doc=The name of the index to upload to. If not specified ensure a column is named '_index'."`
	Type               string              `vfilter:"optional,field=type,doc=The type of the index to upload to."`
	ChunkSize          int64               `vfilter:"optional,field=chunk_size,doc=The number of rows to send at the time."`
	Addresses          []string            `vfilter:"optional,field=addresses,doc=A list of Elasticsearch nodes to use."`
	Username           string              `vfilter:"optional,field=username,doc=Username for HTTP Basic Authentication."`
	Password           string              `vfilter:"optional,field=password,doc=Password for HTTP Basic Authentication."`
	CloudID            string              `vfilter:"optional,field=cloud_id,doc=Endpoint for the Elastic Service (https://elastic.co/cloud)."`
	APIKey             string              `vfilter:"optional,field=api_key,doc=Base64-encoded token for authorization; if set, overrides username and password."`
	WaitTime           int64               `vfilter:"optional,field=wait_time,doc=Batch elastic upload this long (2 sec)."`
	PipeLine           string              `vfilter:"optional,field=pipeline,doc=Pipeline for uploads"`
	DisableSSLSecurity bool                `vfilter:"optional,field=disable_ssl_security,doc=Disable ssl certificate verifications (deprecated in favor of SkipVerify)."`
	SkipVerify         bool                `vfilter:"optional,field=skip_verify,doc=Disable ssl certificate verifications."`
	RootCerts          string              `vfilter:"optional,field=root_ca,doc=As a better alternative to disable_ssl_security, allows root ca certs to be added here."`
	MaxMemoryBuffer    uint64              `vfilter:"optional,field=max_memory_buffer,doc=How large we allow the memory buffer to grow to while we are trying to contact the Elastic server (default 100mb)."`
	Action             string              `vfilter:"optional,field=action,doc=Either index or create. For data streams this must be create."`
	Secret             string              `vfilter:"optional,field=secret,doc=Alternatively use a secret from the secrets service. Secret must be of type 'AWS S3 Creds'"`
}

type _ElasticPlugin struct{}

func (self _ElasticPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "elastic", args)()

		err := vql_subsystem.CheckAccess(scope, acls.NETWORK)
		if err != nil {
			scope.Log("elastic: %v", err)
			return
		}

		arg := &_ElasticPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("elastic: %v", err)
			return
		}

		err = self.maybeForceSecrets(ctx, scope, arg)
		if err != nil {
			scope.Log("elastic: %v", err)
			return
		}

		if arg.Secret != "" {
			err := mergeSecretElastic(ctx, scope, arg)
			if err != nil {
				scope.Log("elastic_upload: %v", err)
				return
			}
		}

		if arg.Action == "" {
			arg.Action = "index"
		}
		if arg.Action != "index" && arg.Action != "create" {
			scope.Log("elastic: action must be either index or create")
			return
		}

		config_obj, _ := artifacts.GetConfig(scope)

		if arg.Threads == 0 {
			arg.Threads = 1
		}

		if arg.ChunkSize == 0 {
			arg.ChunkSize = 1000
		}

		if arg.WaitTime == 0 {
			arg.WaitTime = 2
		}

		wg := sync.WaitGroup{}
		row_chan := arg.Query.Eval(ctx, scope)
		for i := 0; i < int(arg.Threads); i++ {
			wg.Add(1)

			// Separate the IDs from each thread.
			id := time.Now().UnixNano() + int64(i)*100000000

			// Start an uploader on a thread.
			go upload_rows(ctx, config_obj, scope, output_chan,
				row_chan, id, arg.Action, &wg, arg)
		}

		wg.Wait()
	}()
	return output_chan
}

// Copy rows from row_chan to a local buffer and push it up to elastic.
func upload_rows(
	ctx context.Context,
	config_obj *config_proto.ClientConfig,
	scope vfilter.Scope, output_chan chan vfilter.Row,
	row_chan <-chan vfilter.Row,
	id int64, action string,
	wg *sync.WaitGroup,
	arg *_ElasticPluginArgs) {
	defer wg.Done()

	var buf bytes.Buffer

	tlsConfig, err := networking.GetTlsConfig(config_obj, arg.RootCerts)
	if err != nil {
		scope.Log("elastic: cannot get TLS config: %s", err)
		return
	}

	if arg.DisableSSLSecurity || arg.SkipVerify {
		if arg.DisableSSLSecurity {
			scope.Log("elastic: DisableSSLSecurity is deprecated, please use SkipVerify instead")
		}

		if err = networking.EnableSkipVerify(tlsConfig, config_obj); err != nil {
			scope.Log("elastic: cannot disable SSL security: %s", err)
			return
		}
	}

	cfg := elasticsearch.Config{
		Addresses: arg.Addresses,
		Username:  arg.Username,
		Password:  arg.Password,
		CloudID:   arg.CloudID,
		APIKey:    arg.APIKey,
		Transport: &http.Transport{
			MaxIdleConnsPerHost:   10,
			ResponseHeaderTimeout: 100 * time.Second,
			TLSClientConfig:       tlsConfig,
		},
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		scope.Log("elastic: %v", err)
		return
	}

	wait_time := time.Duration(arg.WaitTime) * time.Second
	next_send_time := time.After(wait_time)

	// If the buffer is too large we need to drop the data on the
	// floor. This might happen if the elastic server is not reachable
	// for example.
	max_buffer_size := uint64(100 * 1024 * 1024)
	if arg.MaxMemoryBuffer > 0 {
		max_buffer_size = arg.MaxMemoryBuffer
	}

	// Flush any remaining rows
	defer send_to_elastic(ctx, scope, output_chan, client, &buf)

	opts := vql_subsystem.EncOptsFromScope(scope)

	count := int64(0)

	// Batch sending to elastic: Either
	// when we get to chuncksize or wait
	// time whichever comes first.
	for {
		select {
		case row, ok := <-row_chan:
			if !ok {
				return
			}

			id += int64(utils.GetId())
			err := append_row_to_buffer(ctx, scope, action, row,
				id, &buf, arg, opts)
			if err != nil {
				scope.Log("elastic: %v", err)
				continue
			}

			count++

			if count > arg.ChunkSize ||
				buf.Len() > int(max_buffer_size) {
				send_to_elastic(ctx, scope, output_chan, client, &buf)
				count = 0
				next_send_time = time.After(wait_time)
			}

		case <-next_send_time:
			send_to_elastic(ctx, scope, output_chan, client, &buf)
			count = 0
			next_send_time = time.After(wait_time)
		}
	}
}

func append_row_to_buffer(
	ctx context.Context,
	scope vfilter.Scope,
	action string,
	row vfilter.Row, id int64, buf *bytes.Buffer,
	arg *_ElasticPluginArgs, opts *json.EncOpts) error {

	row_dict := vfilter.RowToDict(ctx, scope, row)
	index := arg.Index
	index_any, pres := row_dict.Get("_index")
	if pres {
		index = sanitize_index(
			fmt.Sprintf("%v", index_any))
		row_dict.Delete("_index")
	}

	// Allow the user to specify the elastic document ID as the _id
	// column.
	_id, pres := row_dict.GetString("_id")
	if pres {
		row_dict.Delete("_id")
	} else {
		_id = fmt.Sprintf("%v", id)
	}

	var meta []byte
	pipeline := arg.PipeLine
	if pipeline != "" {
		meta = []byte(fmt.Sprintf(`{ %q : {"_id" : "%s", "_index": %q, "pipeline": %q } }%s`,
			action, _id, index, pipeline, "\n"))
	} else {
		meta = []byte(fmt.Sprintf(`{ %q : {"_id" : "%s", "_index": %q} }%s`,
			action, _id, index, "\n"))
	}

	data, err := json.MarshalWithOptions(row_dict, opts)
	if err != nil {
		return err
	}

	data = append(data, "\n"...)

	buf.Grow(len(meta) + len(data))
	buf.Write(meta)
	buf.Write(data)

	return nil
}

func send_to_elastic(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	client *elasticsearch.Client, buf *bytes.Buffer) {
	b := buf.Bytes()
	if len(b) == 0 {
		return
	}

	res, err := client.Bulk(bytes.NewReader(b))
	if err != nil && !errors.Is(err, io.EOF) {
		scope.Log("elastic: %v", err)
		return
	}

	if res == nil {
		scope.Log("elastic: %v", err)
		return
	}

	var response *ordereddict.Dict
	b1, err := utils.ReadAllWithLimit(res.Body, constants.MAX_MEMORY)
	if err == nil {
		response, err = utils.ParseJsonToObject(b1)
		if err != nil {
			return
		}
	}

	select {
	case <-ctx.Done():
		return

	case output_chan <- ordereddict.NewDict().
		Set("StatusCode", res.StatusCode).
		Set("Response", response):
	}

	buf.Reset()

}

// Valid Elastic index names

// https://github.com/elastic/elasticsearch/blob/f6a05c6a7c15deaa583b2054175f81cfa8dca7ac/server/src/main/java/org/elasticsearch/common/Strings.java#L287
func sanitize_index(name string) string {

	// must not be '.' or '..'
	switch name {
	case ".", "..":
		return "invalid"
	}

	res := []rune{}

	// https://github.com/elastic/elasticsearch/blob/608a61ab85e82f8f6e88002ba7d8458411e7da62/core/src/test/java/org/elasticsearch/cluster/metadata/MetaDataCreateIndexServiceTests.java#L188-L202
	for i, c := range name {
		switch c {
		//  must not contain the following characters
		case '\\', '/', '*', '?', '"', '<', '>', '|', ' ', ',', '#':
			res = append(res, '_')
			continue

			// must not start with '_', '-', or '+'
		case '-', '+', '_':
			if i == 0 {
				continue
			}
		}

		res = append(res, c)
	}

	return strings.ToLower(string(res))
}

func (self _ElasticPlugin) maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope, arg *_ElasticPluginArgs) error {

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

func mergeSecretElastic(ctx context.Context, scope vfilter.Scope, arg *_ElasticPluginArgs) error {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return errors.New("elastic_upload: Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	s, err := secrets_service.GetSecret(ctx, principal,
		constants.ELASTIC_CREDS, arg.Secret)
	if err != nil {
		return err
	}

	arg.Addresses = s.GetStrings("addresses")

	if arg.Addresses == nil {
		return errors.New("No addresses present in elastic secret!")
	}

	// Allow the user to override the index
	s.UpdateString("index", &arg.Index)
	s.UpdateString("type", &arg.Type)
	s.UpdateString("pipeline", &arg.PipeLine)
	s.UpdateString("action", &arg.Action)

	arg.Username = s.GetString("username")
	arg.Password = s.GetString("password")
	arg.CloudID = s.GetString("cloud_id")
	arg.APIKey = s.GetString("api_key")
	arg.SkipVerify = s.GetBool("skip_verify")
	arg.RootCerts = s.GetString("root_ca")

	return nil
}

func (self _ElasticPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "elastic_upload",
		Doc:      "Upload rows to elastic.",
		Metadata: vql.VQLMetadata().Permissions(acls.NETWORK).Build(),
		ArgType:  type_map.AddType(scope, &_ElasticPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ElasticPlugin{})
}
