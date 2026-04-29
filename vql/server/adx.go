//go:build sumo
// +build sumo

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
Plugin ADX.
*/
package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Azure/azure-kusto-go/azkustodata"
	"github.com/Azure/azure-kusto-go/azkustoingest"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	networking "www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _ADXPluginArgs struct {
	Query           vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for rows to upload."`
	Threads         int64               `vfilter:"optional,field=threads,doc=How many threads to use."`
	ClusterURL      string              `vfilter:"optional,field=cluster_url,doc=The ADX cluster URL."`
	Database        string              `vfilter:"optional,field=database,doc=The ADX database name."`
	Table           string              `vfilter:"optional,field=table,doc=The name of the table to upload to."`
	ClientID        string              `vfilter:"optional,field=client_id,doc=Azure Service Principal Client ID."`
	ClientSecret    string              `vfilter:"optional,field=client_secret,doc=Azure Service Principal Client Secret."`
	TenantID        string              `vfilter:"optional,field=tenant_id,doc=Azure Service Principal Tenant ID."`
	ChunkSize       int64               `vfilter:"optional,field=chunk_size,doc=The number of rows to send at the time."`
	WaitTime        int64               `vfilter:"optional,field=wait_time,doc=Batch ADX upload this long (5 sec)."`
	MaxMemoryBuffer uint64              `vfilter:"optional,field=max_memory_buffer,doc=How large we allow the memory buffer to grow to while we are trying to contact the ADX server (default 100mb)."`
	Secret          string              `vfilter:"optional,field=secret,doc=Alternatively use a secret from the secrets service. Secret must be of type 'ADX Creds'"`
}

type _ADXPlugin struct{}

func (self _ADXPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "adx_upload", args)()

		err := vql_subsystem.CheckAccess(scope, acls.NETWORK)
		if err != nil {
			scope.Log("adx_upload: %v", err)
			return
		}

		arg := &_ADXPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("adx_upload: %v", err)
			return
		}

		err = self.maybeForceSecrets(ctx, scope, arg)
		if err != nil {
			scope.Log("adx_upload: %v", err)
			return
		}

		if arg.Secret != "" {
			err := mergeSecretADX(ctx, scope, arg)
			if err != nil {
				scope.Log("adx_upload: %v", err)
				return
			}
		}

		if arg.ClusterURL == "" {
			scope.Log("adx_upload: field cluster_url is required")
			return
		}

		// Validate ClusterURL format
		_, err = url.Parse(arg.ClusterURL)
		if err != nil {
			scope.Log("adx_upload: invalid cluster_url format: %v", err)
			return
		}

		if arg.Database == "" {
			scope.Log("adx_upload: field database is required")
			return
		}

		if arg.ClientID == "" || arg.ClientSecret == "" || arg.TenantID == "" {
			scope.Log("adx_upload: client_id, client_secret, and tenant_id are required")
			return
		}

		if arg.Threads <= 0 {
			arg.Threads = 1
		}

		if arg.ChunkSize <= 0 {
			arg.ChunkSize = 1000
		}

		if arg.WaitTime <= 0 {
			arg.WaitTime = 5
		}

		if arg.Table == "" {
			arg.Table = "RawVelociraptorEvents"
		}

		// Set default max memory buffer if not specified
		if arg.MaxMemoryBuffer == 0 {
			arg.MaxMemoryBuffer = 100 * 1024 * 1024 // 100MB default
		}

		config_obj, _ := artifacts.GetConfig(scope)

		wg := sync.WaitGroup{}
		row_chan := arg.Query.Eval(ctx, scope)
		for i := 0; i < int(arg.Threads); i++ {
			wg.Add(1)

			// Start an uploader on a thread.
			go _upload_rows_adx(ctx, scope, config_obj, output_chan,
				row_chan, &wg, arg)
		}

		wg.Wait()
	}()
	return output_chan
}

// Copy rows from row_chan to a local buffer and push it up to ADX.
func _upload_rows_adx(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.ClientConfig,
	output_chan chan vfilter.Row,
	row_chan <-chan vfilter.Row,
	wg *sync.WaitGroup,
	arg *_ADXPluginArgs) {
	defer wg.Done()

	var rowCount int64
	var jsonBuf bytes.Buffer

	// Track ingestion time & start time per batch so all rows flushed together share the same timestamp/metrics.
	batchIngestionTime := ""
	var batchStart time.Time

	// Create connection string with service principal auth
	kustoConnectionString := azkustodata.NewConnectionStringBuilder(arg.ClusterURL).
		WithAadAppKey(arg.ClientID, arg.ClientSecret, arg.TenantID)

	// Azure Kusto ingest-* endpoints (e.g. ingest-*.kusto.windows.net) do not
	// support TLS 1.3, therefore force TLS 1.2 for now
	transport, err := networking.GetNewHttpTransport(config_obj, "")
	if err != nil {
		scope.Log("adx_upload: failed to create http transport: %v", err)
		return
	}
	transport.TLSClientConfig.MinVersion = tls.VersionTLS12
	transport.TLSClientConfig.MaxVersion = tls.VersionTLS12
	httpClient := &http.Client{Transport: transport}

	// Create managed ingest client (handles streaming/queued automatically)
	ingestClient, err := azkustoingest.New(
		kustoConnectionString,
		azkustoingest.WithDefaultDatabase(arg.Database),
		azkustoingest.WithDefaultTable(arg.Table),
		azkustoingest.WithHttpClient(httpClient))
	if err != nil {
		scope.Log("adx_upload: failed to create ingest client: %v", err)
		return
	}
	defer ingestClient.Close()

	flushBuffer := func(reason string) {
		if rowCount == 0 || jsonBuf.Len() == 0 {
			jsonBuf.Reset()
			batchIngestionTime = ""
			batchStart = time.Time{}
			return
		}
		send_to_adx(ctx, scope, output_chan, ingestClient, rowCount, &jsonBuf, arg, reason, batchStart)
		rowCount = 0
		batchIngestionTime = ""
		batchStart = time.Time{}
	}
	defer func() {
		flushBuffer("shutdown")
	}()

	wait_time := time.Duration(arg.WaitTime) * time.Second
	timer := time.NewTimer(wait_time)
	defer timer.Stop()

	// resetTimer safely drains and resets the timer.
	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(wait_time)
	}

	opts := vql_subsystem.EncOptsFromScope(scope)

	// Batch sending to ADX: Either when we get to chunksize or
	// wait time whichever comes first.
	for {
		select {
		case <-ctx.Done():
			flushBuffer("context_cancelled")
			return

		case row, ok := <-row_chan:
			if !ok {
				flushBuffer("channel_closed")
				return
			}

			// Check if buffer is getting too large before processing
			if int64(jsonBuf.Len()) > int64(arg.MaxMemoryBuffer) {
				scope.Log("adx_upload: buffer exceeded max_memory_buffer (%d bytes), forcing send", arg.MaxMemoryBuffer)
				flushBuffer("max_memory_buffer")
				resetTimer()
			}

			// Set ingestion time once per batch
			if batchIngestionTime == "" {
				batchIngestionTime = time.Now().UTC().Format(time.RFC3339)
				batchStart = time.Now()
			}

			// Process row into buffer
			err := append_row_to_adx_buffer(ctx, scope, row, &jsonBuf, batchIngestionTime, opts)
			if err != nil {
				scope.Log("adx_upload: failed to process row: %v", err)
				continue
			}

			// Only count successfully processed rows.
			rowCount++

			// Do not allow the buffer to get too large.
			if rowCount >= arg.ChunkSize {
				flushBuffer("chunk_size")
				resetTimer()
			} else if int64(jsonBuf.Len()) > int64(arg.MaxMemoryBuffer) {
				flushBuffer("max_memory_buffer")
				resetTimer()
			}

		case <-timer.C:
			flushBuffer("wait_time")
			timer.Reset(wait_time)
		}
	}
}

func append_row_to_adx_buffer(
	ctx context.Context,
	scope vfilter.Scope,
	row vfilter.Row,
	jsonBuf *bytes.Buffer,
	ingestionTime string,
	opts *json.EncOpts) error {

	// Check context cancellation before expensive operations
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	vql_row_dict := vfilter.RowToDict(ctx, scope, row)

	// Extract metadata fields (prefixed with _ to avoid collisions with artifact columns)
	artifact, _ := vql_row_dict.GetString("_Artifact")
	clientId, _ := vql_row_dict.GetString("_ClientId")
	flowId, _ := vql_row_dict.GetString("_FlowId")
	organization, _ := vql_row_dict.GetString("_Organization")
	hostname, _ := vql_row_dict.GetString("_Hostname")

	// Prefer artifact's own Timestamp column if present, otherwise fall back to injected _timestamp
	timestamp, _ := vql_row_dict.Get("Timestamp")
	if timestamp == nil {
		timestamp, _ = vql_row_dict.Get("_timestamp")
	}

	// Remove metadata fields from the row before storing it as RawData
	vql_row_dict.Delete("_Artifact")
	vql_row_dict.Delete("_ClientId")
	vql_row_dict.Delete("_FlowId")
	vql_row_dict.Delete("_Organization")
	vql_row_dict.Delete("_Hostname")
	vql_row_dict.Delete("_timestamp")

	rawData, err := json.MarshalWithOptions(vql_row_dict, opts)
	if err != nil {
		return err
	}

	jsonBuf.WriteString(json.Format(
		`{"IngestionTime":%q,"Artifact":%q,"ClientId":%q,"FlowId":%q,"Organization":%q,"Hostname":%q,"Timestamp":%q,"RawData":%s}`+"\n",
		ingestionTime, artifact, clientId, flowId, organization, hostname, timestamp, rawData,
	))

	return nil
}

func send_to_adx(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	ingestClient *azkustoingest.Ingestion,
	rowCount int64,
	jsonBuf *bytes.Buffer,
	arg *_ADXPluginArgs,
	flushReason string,
	batchStart time.Time) {

	if rowCount == 0 || jsonBuf.Len() == 0 {
		return
	}

	dataSize := jsonBuf.Len()
	rowsProcessed := rowCount
	var duration time.Duration
	if !batchStart.IsZero() {
		duration = time.Since(batchStart)
	}

	// If the caller's context is already cancelled (e.g. final flush on shutdown),
	// use a short-lived background context so the last batch is not dropped.
	uploadCtx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		uploadCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	// Upload to ADX
	result, err := ingestClient.FromReader(
		uploadCtx,
		bytes.NewReader(jsonBuf.Bytes()),
		azkustoingest.FileFormat(azkustoingest.JSON),
	)

	// Clear the buffer after sending
	jsonBuf.Reset()

	if err != nil {
		scope.Log("adx_upload: failed to upload %d rows (%d bytes) (reason=%s duration=%v): %v",
			rowsProcessed, dataSize, flushReason, duration, err)
		select {
		case <-uploadCtx.Done():
			return
		case output_chan <- ordereddict.NewDict().
			Set("Rows", rowsProcessed).
			Set("Bytes", dataSize).
			Set("DurationMs", duration.Milliseconds()).
			Set("FlushReason", flushReason).
			Set("Status", "Error").
			Set("Response", err.Error()):
		}
	} else {
		statusMsg := "Ingestion queued successfully"
		if result != nil {
			// Log that we got a result (though we don't wait for completion)
			statusMsg = "Ingestion queued successfully (async)"
		}
		scope.Log("adx_upload: successfully uploaded %d rows (%d bytes) to %s.%s (reason=%s duration=%v)",
			rowsProcessed, dataSize, arg.Database, arg.Table, flushReason, duration)
		select {
		case <-uploadCtx.Done():
			return
		case output_chan <- ordereddict.NewDict().
			Set("Rows", rowsProcessed).
			Set("Bytes", dataSize).
			Set("DurationMs", duration.Milliseconds()).
			Set("FlushReason", flushReason).
			Set("Status", "Success").
			Set("Database", arg.Database).
			Set("Table", arg.Table).
			Set("Response", statusMsg):
		}
	}
}

func (self _ADXPlugin) maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope, arg *_ADXPluginArgs) error {

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

func mergeSecretADX(ctx context.Context, scope vfilter.Scope, arg *_ADXPluginArgs) error {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return errors.New("adx_upload: Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	s, err := secrets_service.GetSecret(ctx, principal,
		constants.ADX_CREDS, arg.Secret)
	if err != nil {
		return err
	}

	// Allow the user to override these fields
	s.UpdateString("table", &arg.Table)
	s.UpdateString("cluster_url", &arg.ClusterURL)
	s.UpdateString("database", &arg.Database)

	// These always come from secret (credentials)
	arg.ClientID = s.GetString("client_id")
	arg.ClientSecret = s.GetString("client_secret")
	arg.TenantID = s.GetString("tenant_id")

	return nil
}

func (self _ADXPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "adx_upload",
		Doc:      "Upload rows to Azure Data Explorer (ADX).",
		ArgType:  type_map.AddType(scope, &_ADXPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.NETWORK).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ADXPlugin{})
}
