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

/* Plugin Elastic.


 */
package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	elasticsearch "github.com/Velocidex/go-elasticsearch/v7"
	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
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
	DisableSSLSecurity bool                `vfilter:"optional,field=disable_ssl_security,doc=Disable ssl certificate verifications."`
	RootCerts          string              `vfilter:"optional,field=root_ca,doc=As a better alternative to disable_ssl_security, allows root ca certs to be added here."`
}

type _ElasticPlugin struct{}

func (self _ElasticPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
		if err != nil {
			scope.Log("elastic: %v", err)
			return
		}

		arg := _ElasticPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, &arg)
		if err != nil {
			scope.Log("elastic: %v", err)
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
				row_chan, id, &wg, &arg)
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
	id int64,
	wg *sync.WaitGroup,
	arg *_ElasticPluginArgs) {
	defer wg.Done()

	var buf bytes.Buffer

	CA_Pool := x509.NewCertPool()
	crypto.AddPublicRoots(CA_Pool)
	err := crypto.AddDefaultCerts(config_obj, CA_Pool)
	if err != nil {
		scope.Log("elastic: %v", err)
		return
	}

	if arg.RootCerts != "" &&
		!CA_Pool.AppendCertsFromPEM([]byte(arg.RootCerts)) {
		scope.Log("elastic: Unable to add root certs")
		return
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
			TLSClientConfig: &tls.Config{
				ClientSessionCache: tls.NewLRUClientSessionCache(100),
				RootCAs:            CA_Pool,
				InsecureSkipVerify: arg.DisableSSLSecurity,
			},
		},
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		scope.Log("elastic: %v", err)
		return
	}

	wait_time := time.Duration(arg.WaitTime) * time.Second
	next_send_id := id + arg.ChunkSize
	next_send_time := time.After(wait_time)

	// Flush any remaining rows
	defer send_to_elastic(ctx, scope, output_chan, client, &buf)

	opts := vql_subsystem.EncOptsFromScope(scope)

	// Batch sending to elastic: Either
	// when we get to chuncksize or wait
	// time whichever comes first.
	for {
		select {
		case row, ok := <-row_chan:
			if !ok {
				return
			}

			// FIXME: Find a better way to interleave id's
			// to avoid collisions.
			id = id + 3
			err := append_row_to_buffer(ctx, scope, row, id, &buf, arg, opts)
			if err != nil {
				scope.Log("elastic: %v", err)
				continue
			}

			if id > next_send_id {
				send_to_elastic(ctx, scope, output_chan,
					client, &buf)
				next_send_id = id + arg.ChunkSize
				next_send_time = time.After(wait_time)
			}

		case <-next_send_time:
			send_to_elastic(ctx, scope, output_chan,
				client, &buf)
			next_send_id = id + arg.ChunkSize
			next_send_time = time.After(wait_time)
		}
	}
}

func append_row_to_buffer(
	ctx context.Context,
	scope vfilter.Scope,
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

	var meta []byte
	pipeline := arg.PipeLine
	if pipeline != "" {
		meta = []byte(fmt.Sprintf(`{ "index" : {"_id" : "%d", "_index": "%s", "pipeline": "%s" } }%s`,
			id, index, pipeline, "\n"))
	} else {
		meta = []byte(fmt.Sprintf(`{ "index" : {"_id" : "%d", "_index": "%s"} }%s`,
			id, index, "\n"))
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
	if err != nil && errors.Cause(err) != io.EOF {
		scope.Log("elastic: %v", err)
		return
	}

	response := make(map[string]interface{})
	b1, err := ioutil.ReadAll(res.Body)
	if err == nil {
		_ = json.Unmarshal(b1, &response)
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

var sanitize_index_re = regexp.MustCompile("[^a-zA-Z0-9]")

func sanitize_index(name string) string {
	return sanitize_index_re.ReplaceAllLiteralString(
		strings.ToLower(name), "_")
}

func (self _ElasticPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "elastic_upload",
		Doc:  "Upload rows to elastic.",

		ArgType: type_map.AddType(scope, &_ElasticPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ElasticPlugin{})
}
