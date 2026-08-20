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
Plugin Azure Monitor (Log Analytics).

Uploads rows to an Azure Log Analytics workspace using the Azure Monitor Logs
Ingestion API (the modern Data Collection Rule based API which superseded the
deprecated HTTP Data Collector API).

Unlike the ADX plugin (which talks to a Kusto cluster's Data Management endpoint
via the Azure Kusto SDK), Log Analytics does not expose a Kusto ingestion
endpoint. Data is instead POSTed as a JSON array to a Data Collection Rule (DCR)
stream, authenticated with an Entra (Azure AD) OAuth2 bearer token. The service
principal or managed identity must hold the "Monitoring Metrics Publisher" role
on the DCR.

The service-principal auth path uses only the standard library plus
golang.org/x/oauth2 so this file carries no build tag and is compiled into every
build (like elastic.go / splunk.go). The managed-identity path pulls in the
azidentity SDK and therefore lives in azure_monitor_mi.go behind the "sumo" tag.
*/
package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/oauth2/microsoft"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// The OAuth2 scope used to obtain a token for the Logs Ingestion API.
const azureMonitorScope = "https://monitor.azure.com/.default"

// The Logs Ingestion API caps a single request at ~1MB uncompressed. We default
// the buffer well under that to leave headroom for the JSON array framing.
const azureMonitorDefaultMaxBuffer = uint64(900 * 1024)

// The hard ceiling for max_memory_buffer - the documented Logs Ingestion
// request cap. Anything above this is guaranteed to be rejected with a 413.
const azureMonitorMaxBuffer = uint64(1024 * 1024)

// chunk_size sizes an allocation, so it needs an upper bound as well as a lower
// one. A batch must fit under max_memory_buffer anyway, so no legitimate
// configuration comes near this.
const azureMonitorMaxChunkSize = int64(100000)

// Never honor a Retry-After longer than this - a misconfigured (or hostile)
// server should not be able to park an upload worker for hours.
const azureMonitorMaxRetryAfter = 5 * time.Minute

// azureTokenSourceFunc builds an OAuth2 token source from the Azure identity
// SDK. The managed-identity and default-credential implementations pull in the
// azidentity SDK and are therefore provided by an init() in azure_monitor_mi.go,
// compiled only under the "sumo" build tag. In builds without that tag these
// modes are unavailable and the stubs below return an explanatory error.
//
// Note the plain service-principal path (including AZURE_* environment variable
// credentials) does NOT use these - it uses golang.org/x/oauth2 directly and is
// always compiled.
type azureTokenSourceFunc func(
	ctx context.Context, scope vfilter.Scope,
	transport *http.Transport,
	arg *_AzureMonitorPluginArgs) (oauth2.TokenSource, error)

var azureMonitorMITokenSource azureTokenSourceFunc = func(
	ctx context.Context, scope vfilter.Scope,
	transport *http.Transport,
	arg *_AzureMonitorPluginArgs) (oauth2.TokenSource, error) {
	return nil, errors.New(
		"managed_identity requires a Velociraptor build with the 'sumo' tag")
}

var azureMonitorDefaultTokenSource azureTokenSourceFunc = func(
	ctx context.Context, scope vfilter.Scope,
	transport *http.Transport,
	arg *_AzureMonitorPluginArgs) (oauth2.TokenSource, error) {
	return nil, errors.New(
		"default_credential requires a Velociraptor build with the 'sumo' tag")
}

type _AzureMonitorPluginArgs struct {
	Query                 vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for rows to upload."`
	Threads               int64               `vfilter:"optional,field=threads,doc=How many threads to use."`
	LogsIngestionEndpoint string              `vfilter:"optional,field=logs_ingestion_endpoint,doc=The Logs Ingestion endpoint URI of the Data Collection Rule (or a Data Collection Endpoint)."`
	DCRImmutableID        string              `vfilter:"optional,field=dcr_immutable_id,doc=The immutable ID of the Data Collection Rule (e.g. dcr-xxxxxxxx)."`
	StreamName            string              `vfilter:"optional,field=stream_name,doc=The DCR input stream name (e.g. Custom-RawVelociraptorEvents_CL). If empty it is derived from table as 'Custom-<table>'."`
	Table                 string              `vfilter:"optional,field=table,doc=Used to derive stream_name as 'Custom-<table>' when stream_name is not set. Unused otherwise."`
	TenantID              string              `vfilter:"optional,field=tenant_id,doc=Azure Service Principal Tenant ID."`
	ClientID              string              `vfilter:"optional,field=client_id,doc=Azure Service Principal Client ID."`
	ClientSecret          string              `vfilter:"optional,field=client_secret,doc=Azure Service Principal Client Secret."`
	ManagedIdentity       bool                `vfilter:"optional,field=managed_identity,doc=Use an Azure managed identity instead of a service principal (requires a 'sumo' build and that the server runs in Azure)."`
	ManagedIdentityClient string              `vfilter:"optional,field=managed_identity_client_id,doc=Optional client ID of a user-assigned managed identity."`
	DefaultCredential     bool                `vfilter:"optional,field=default_credential,doc=Use the Azure SDK DefaultAzureCredential chain (AZURE_* env vars, workload identity, managed identity, Azure CLI). Requires a 'sumo' build."`
	ChunkSize             int64               `vfilter:"optional,field=chunk_size,doc=The number of rows to send at a time."`
	WaitTime              int64               `vfilter:"optional,field=wait_time,doc=Batch upload at most this long in seconds (default 5)."`
	MaxMemoryBuffer       uint64              `vfilter:"optional,field=max_memory_buffer,doc=Max uncompressed request body in bytes; keep under the ~1MB Azure limit (default 900KB)."`
	SkipVerify            bool                `vfilter:"optional,field=skip_verify,doc=Skip TLS verification (default: False)."`
	RootCerts             string              `vfilter:"optional,field=root_ca,doc=As a better alternative to skip_verify, allows root ca certs to be added here."`
	MaxRetries            int64               `vfilter:"optional,field=max_retries,doc=Maximum number of retries for failed uploads (default: 3)."`
	RetryWait             int64               `vfilter:"optional,field=retry_wait,doc=Base wait time in seconds for exponential backoff between retries (default: 2)."`
	Secret                string              `vfilter:"optional,field=secret,doc=Alternatively use a secret from the secrets service. Secret must be of type 'Azure Monitor Creds'."`
}

type _AzureMonitorPlugin struct{}

func (self _AzureMonitorPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "azure_monitor_upload", args)()

		err := vql_subsystem.CheckAccess(scope, acls.NETWORK)
		if err != nil {
			scope.Log("azure_monitor_upload: %v", err)
			return
		}

		arg := &_AzureMonitorPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("azure_monitor_upload: %v", err)
			return
		}

		err = self.maybeForceSecrets(ctx, scope, arg)
		if err != nil {
			scope.Log("azure_monitor_upload: %v", err)
			return
		}

		if arg.Secret != "" {
			err := mergeSecretAzureMonitor(ctx, scope, arg)
			if err != nil {
				scope.Log("azure_monitor_upload: %v", err)
				return
			}
		}

		if arg.LogsIngestionEndpoint == "" {
			scope.Log("azure_monitor_upload: field logs_ingestion_endpoint is required")
			return
		}

		// url.Parse accepts almost anything, so check the parts we actually
		// need. A missing host can only ever produce a broken request URL.
		endpoint_url, err := url.Parse(arg.LogsIngestionEndpoint)
		if err != nil {
			scope.Log("azure_monitor_upload: invalid logs_ingestion_endpoint: %v", err)
			return
		}
		if endpoint_url.Host == "" {
			scope.Log("azure_monitor_upload: invalid logs_ingestion_endpoint %q: no host - expected e.g. https://<dce>.<region>.ingest.monitor.azure.com",
				arg.LogsIngestionEndpoint)
			return
		}
		// The oauth2 transport attaches the Entra bearer token to every
		// request, so a non-https endpoint puts a live token on the wire.
		if endpoint_url.Scheme != "https" {
			scope.Log("azure_monitor_upload: logs_ingestion_endpoint %q is not https - the Azure access token will be sent in cleartext",
				arg.LogsIngestionEndpoint)
		}

		if arg.DCRImmutableID == "" {
			scope.Log("azure_monitor_upload: field dcr_immutable_id is required")
			return
		}

		// Resolve the stream name from table if not given explicitly.
		if arg.StreamName == "" {
			if arg.Table == "" {
				scope.Log("azure_monitor_upload: either stream_name or table is required")
				return
			}
			arg.StreamName = "Custom-" + arg.Table
		}

		// Allow credentials to come from the standard Azure environment
		// variables (as the Azure SDKs do by default) when they are not
		// supplied explicitly. This still uses the proxy / CA aware
		// client-credentials flow below.
		if arg.ClientID == "" {
			arg.ClientID = os.Getenv("AZURE_CLIENT_ID")
		}
		if arg.ClientSecret == "" {
			arg.ClientSecret = os.Getenv("AZURE_CLIENT_SECRET")
		}
		if arg.TenantID == "" {
			arg.TenantID = os.Getenv("AZURE_TENANT_ID")
		}

		// Credentials: a managed identity, the default credential chain, or a
		// full service principal (from args, a secret, or AZURE_* env vars).
		if !arg.ManagedIdentity && !arg.DefaultCredential {
			if arg.ClientID == "" || arg.ClientSecret == "" || arg.TenantID == "" {
				scope.Log("azure_monitor_upload: no credentials found - supply client_id/client_secret/tenant_id (or set the AZURE_CLIENT_ID/AZURE_CLIENT_SECRET/AZURE_TENANT_ID env vars), or set managed_identity=TRUE or default_credential=TRUE")
				return
			}
		}

		if arg.Threads <= 0 {
			arg.Threads = 1
		}
		if arg.ChunkSize <= 0 {
			arg.ChunkSize = 1000
		} else if arg.ChunkSize > azureMonitorMaxChunkSize {
			scope.Log("azure_monitor_upload: chunk_size %d exceeds the maximum of %d - clamping",
				arg.ChunkSize, azureMonitorMaxChunkSize)
			arg.ChunkSize = azureMonitorMaxChunkSize
		}
		if arg.WaitTime <= 0 {
			arg.WaitTime = 5
		}
		if arg.MaxRetries <= 0 {
			arg.MaxRetries = 3
		}
		if arg.RetryWait <= 0 {
			arg.RetryWait = 2
		}
		// Note a negative value from VQL arrives here as a huge uint64 (the
		// arg parser converts through int64), so the upper clamp is what
		// rejects it - it must not be allowed to disable the size guards.
		if arg.MaxMemoryBuffer == 0 {
			arg.MaxMemoryBuffer = azureMonitorDefaultMaxBuffer
		} else if arg.MaxMemoryBuffer > azureMonitorMaxBuffer {
			scope.Log("azure_monitor_upload: max_memory_buffer %d exceeds the Azure request limit - clamping to %d",
				arg.MaxMemoryBuffer, azureMonitorMaxBuffer)
			arg.MaxMemoryBuffer = azureMonitorMaxBuffer
		}

		config_obj, _ := artifacts.GetConfig(scope)

		// Build a single transport honoring proxy / CA / skip_verify
		// settings, shared by all upload threads. GetNewHttpTransport
		// returns a clone, so it is safe to mutate its TLS config (unlike
		// the cached GetHttpTransport).
		transport, err := networking.GetNewHttpTransport(config_obj, arg.RootCerts)
		if err != nil {
			scope.Log("azure_monitor_upload: cannot create http transport: %v", err)
			return
		}

		if arg.SkipVerify {
			if err := networking.EnableSkipVerify(
				transport.TLSClientConfig, config_obj); err != nil {
				scope.Log("azure_monitor_upload: cannot disable SSL security: %v", err)
				return
			}
		}

		// One token source shared by all threads - the underlying sources
		// cache and refresh tokens safely across goroutines. Creating it up
		// front also fails fast (before the source query starts) when
		// managed_identity or default_credential is requested on a build
		// without the 'sumo' tag.
		tokenSource, err := getAzureMonitorTokenSource(ctx, scope, transport, arg)
		if err != nil {
			scope.Log("azure_monitor_upload: %v", err)
			return
		}

		// The oauth2 transport injects (and transparently refreshes) the
		// bearer token, and sends the data request through our Base
		// transport.
		httpClient := &http.Client{
			Timeout: 60 * time.Second,
			Transport: &oauth2.Transport{
				Base:   transport,
				Source: tokenSource,
			},
		}

		uploadURL := buildIngestionURL(
			arg.LogsIngestionEndpoint, arg.DCRImmutableID, arg.StreamName)

		wg := sync.WaitGroup{}
		row_chan := arg.Query.Eval(ctx, scope)
		for i := 0; i < int(arg.Threads); i++ {
			wg.Add(1)

			// Start an uploader on a thread.
			go _upload_rows_azure(ctx, scope, output_chan,
				row_chan, &wg, httpClient, uploadURL, arg)
		}

		wg.Wait()
	}()
	return output_chan
}

// Copy rows from row_chan into a local batch and POST them to the Logs
// Ingestion API.
func _upload_rows_azure(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	row_chan <-chan vfilter.Row,
	wg *sync.WaitGroup,
	httpClient *http.Client,
	uploadURL string,
	arg *_AzureMonitorPluginArgs) {
	defer wg.Done()

	var rowCount int64
	var byteEstimate int
	buf := make([][]byte, 0, arg.ChunkSize)
	var batchStart time.Time

	opts := vql_subsystem.EncOptsFromScope(scope)

	flushBuffer := func(reason string) {
		if rowCount > 0 {
			send_to_azure_monitor(ctx, scope, output_chan, httpClient,
				uploadURL, buf, rowCount, byteEstimate, arg, reason, batchStart)
		}
		buf = buf[:0]
		rowCount = 0
		byteEstimate = 0
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

	// Batch sending: flush when we reach chunk_size, when the buffer would
	// exceed max_memory_buffer, or when wait_time elapses - whichever first.
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

			data, err := marshal_azure_row(ctx, scope, row, opts)
			if err != nil {
				scope.Log("azure_monitor_upload: failed to process row: %v", err)
				continue
			}

			// A single row larger than the request cap could never be
			// ingested - drop it with a warning rather than wedging the
			// worker into endless rejections.
			if uint64(len(data)) > arg.MaxMemoryBuffer {
				scope.Log("azure_monitor_upload: dropping oversized row (%d bytes > max_memory_buffer %d)",
					len(data), arg.MaxMemoryBuffer)
				continue
			}

			// Flush first if appending this row would exceed the cap.
			if rowCount > 0 &&
				uint64(byteEstimate+len(data)+1) > arg.MaxMemoryBuffer {
				flushBuffer("max_memory_buffer")
				resetTimer()
			}

			if batchStart.IsZero() {
				batchStart = time.Now()
			}

			buf = append(buf, data)
			byteEstimate += len(data) + 1 // +1 for the array separator
			rowCount++

			if rowCount >= arg.ChunkSize {
				flushBuffer("chunk_size")
				resetTimer()
			}

		case <-timer.C:
			flushBuffer("wait_time")
			timer.Reset(wait_time)
		}
	}
}

// marshal_azure_row shapes a VQL row into the JSON object expected by the DCR
// stream. Every Log Analytics table requires a TimeGenerated column, so we
// always populate it. The remaining metadata is lifted into structured columns
// and the original row is preserved under RawData.
func marshal_azure_row(
	ctx context.Context,
	scope vfilter.Scope,
	row vfilter.Row,
	opts *json.EncOpts) ([]byte, error) {

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	row_dict := vfilter.RowToDict(ctx, scope, row)

	// Extract metadata fields (prefixed with _ to avoid collisions with
	// artifact columns).
	artifact, _ := row_dict.GetString("_Artifact")
	clientId, _ := row_dict.GetString("_ClientId")
	flowId, _ := row_dict.GetString("_FlowId")
	organization, _ := row_dict.GetString("_Organization")
	hostname, _ := row_dict.GetString("_Hostname")

	// Prefer the artifact's own Timestamp column, fall back to the injected
	// _timestamp, otherwise default to now.
	timeGenerated := ""
	if ts, pres := row_dict.Get("Timestamp"); pres && !utils.IsNil(ts) {
		timeGenerated = formatAzureTime(ctx, scope, ts)
	}
	if timeGenerated == "" {
		if ts, pres := row_dict.Get("_timestamp"); pres && !utils.IsNil(ts) {
			timeGenerated = formatAzureTime(ctx, scope, ts)
		}
	}
	if timeGenerated == "" {
		timeGenerated = time.Now().UTC().Format(time.RFC3339Nano)
	}

	// Remove the injected metadata fields before storing the row as RawData
	// (but keep the artifact's own Timestamp column intact).
	row_dict.Delete("_Artifact")
	row_dict.Delete("_ClientId")
	row_dict.Delete("_FlowId")
	row_dict.Delete("_Organization")
	row_dict.Delete("_Hostname")
	row_dict.Delete("_timestamp")

	out := ordereddict.NewDict().
		Set("TimeGenerated", timeGenerated).
		Set("Artifact", artifact).
		Set("ClientId", clientId).
		Set("FlowId", flowId).
		Set("Organization", organization).
		Set("Hostname", hostname).
		Set("RawData", row_dict)

	return json.MarshalWithOptions(out, opts)
}

func formatAzureTime(
	ctx context.Context, scope vfilter.Scope, ts interface{}) string {
	t, err := functions.TimeFromAny(ctx, scope, ts)

	// TimeFromAny returns the zero time with a *nil* error for an empty
	// string, for a zero epoch, and for any repeat of an unparseable string
	// (ParseTimeFromString caches the failed parse and later returns it as a
	// hit). Checking err alone would let those through as year 0001, which
	// Azure silently drops - report "no timestamp" so the caller falls back.
	if err != nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func send_to_azure_monitor(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	httpClient *http.Client,
	uploadURL string,
	buf [][]byte,
	rowCount int64,
	byteEstimate int,
	arg *_AzureMonitorPluginArgs,
	flushReason string,
	batchStart time.Time) {

	if rowCount == 0 || len(buf) == 0 {
		return
	}

	// Assemble the JSON array body.
	var body bytes.Buffer
	body.Grow(byteEstimate + 2)
	body.WriteByte('[')
	for i, r := range buf {
		if i > 0 {
			body.WriteByte(',')
		}
		body.Write(r)
	}
	body.WriteByte(']')

	uncompressedSize := body.Len()

	// gzip the payload to reduce egress. Note the ~1MB Azure limit is on the
	// uncompressed body, which we already bound via max_memory_buffer.
	var gzBuf bytes.Buffer
	gz := gzip.NewWriter(&gzBuf)
	_, err := gz.Write(body.Bytes())
	if err == nil {
		err = gz.Close()
	}
	if err != nil {
		scope.Log("azure_monitor_upload: gzip failed: %v", err)
		return
	}

	var duration time.Duration
	if !batchStart.IsZero() {
		duration = time.Since(batchStart)
	}

	// If the caller's context is already cancelled (final flush on shutdown),
	// use a short-lived background context so the last batch is not dropped.
	uploadCtx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		uploadCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	status, respBody, err := post_azure_batch(
		uploadCtx, scope, httpClient, uploadURL, gzBuf.Bytes(), arg)

	if err != nil {
		scope.Log("azure_monitor_upload: failed to upload %d rows (%d bytes) (reason=%s duration=%v): %v",
			rowCount, uncompressedSize, flushReason, duration, err)
		// Guard on the caller's ctx, not uploadCtx: once the query is
		// cancelled nobody reads output_chan, so the result row is dropped
		// instead of blocking until uploadCtx times out.
		select {
		case <-ctx.Done():
		case output_chan <- ordereddict.NewDict().
			Set("Rows", rowCount).
			Set("Bytes", uncompressedSize).
			Set("DurationMs", duration.Milliseconds()).
			Set("FlushReason", flushReason).
			Set("Status", "Error").
			Set("StatusCode", status).
			Set("Response", responseOrError(respBody, err)):
		}
		return
	}

	scope.Log("azure_monitor_upload: uploaded %d rows (%d bytes) to stream %s (reason=%s duration=%v)",
		rowCount, uncompressedSize, arg.StreamName, flushReason, duration)
	select {
	case <-ctx.Done():
	case output_chan <- ordereddict.NewDict().
		Set("Rows", rowCount).
		Set("Bytes", uncompressedSize).
		Set("DurationMs", duration.Milliseconds()).
		Set("FlushReason", flushReason).
		Set("Status", "Success").
		Set("StatusCode", status).
		Set("StreamName", arg.StreamName).
		Set("Response", "Ingestion accepted"):
	}
}

// post_azure_batch performs the HTTPS POST with retry/backoff. It honors
// Retry-After on 429 and retries transient 5xx errors; 4xx client errors are
// returned immediately since they will not fix themselves.
func post_azure_batch(
	ctx context.Context,
	scope vfilter.Scope,
	httpClient *http.Client,
	uploadURL string,
	gzBody []byte,
	arg *_AzureMonitorPluginArgs) (int, string, error) {

	var lastErr error
	var lastStatus int
	var lastBody string

	for attempt := int64(0); attempt <= arg.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(
			ctx, "POST", uploadURL, bytes.NewReader(gzBody))
		if err != nil {
			// A malformed request will never succeed.
			return 0, "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			lastStatus = 0
			lastBody = ""
		} else {
			body, read_err := utils.ReadAllWithLimit(resp.Body, constants.MAX_MEMORY)
			resp.Body.Close()
			lastStatus = resp.StatusCode
			lastBody = string(body)
			if read_err != nil {
				scope.Log("azure_monitor_upload: error reading response body (status %d): %v",
					resp.StatusCode, read_err)
			}

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return resp.StatusCode, lastBody, nil
			}

			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, lastBody)

			// Only retry on throttling (429) or transient server errors.
			if resp.StatusCode != http.StatusTooManyRequests &&
				resp.StatusCode < 500 {
				return resp.StatusCode, lastBody, lastErr
			}

			// Honor Retry-After on 429 if present, but never on the last
			// attempt - sleeping there just parks the worker (and stalls
			// row_chan) before returning the error we already have.
			if resp.StatusCode == http.StatusTooManyRequests &&
				attempt < arg.MaxRetries {
				if wait := parseRetryAfter(resp.Header.Get("Retry-After")); wait > 0 {
					scope.Log("azure_monitor_upload: throttled (429), waiting %v (Retry-After)...", wait)
					if !sleepCtx(ctx, wait) {
						return lastStatus, lastBody, lastErr
					}
					continue
				}
			}
		}

		// Exponential backoff before the next attempt.
		if attempt < arg.MaxRetries {
			backoff := azureMonitorBackoff(arg.RetryWait, attempt)
			scope.Log("azure_monitor_upload: attempt %d failed: %v, retrying in %v...",
				attempt+1, lastErr, backoff)
			if !sleepCtx(ctx, backoff) {
				return lastStatus, lastBody, lastErr
			}
		}
	}

	return lastStatus, lastBody, lastErr
}

// buildIngestionURL constructs the Logs Ingestion API URL for a DCR stream.
func buildIngestionURL(endpoint, dcr, stream string) string {
	return strings.TrimRight(endpoint, "/") +
		"/dataCollectionRules/" + url.PathEscape(dcr) +
		"/streams/" + url.PathEscape(stream) +
		"?api-version=2023-01-01"
}

// getAzureMonitorTokenSource returns an OAuth2 token source for either a service
// principal (client-credentials flow) or a managed identity.
func getAzureMonitorTokenSource(
	ctx context.Context,
	scope vfilter.Scope,
	transport *http.Transport,
	arg *_AzureMonitorPluginArgs) (oauth2.TokenSource, error) {

	if arg.ManagedIdentity {
		return azureMonitorMITokenSource(ctx, scope, transport, arg)
	}

	if arg.DefaultCredential {
		return azureMonitorDefaultTokenSource(ctx, scope, transport, arg)
	}

	conf := &clientcredentials.Config{
		ClientID:     arg.ClientID,
		ClientSecret: arg.ClientSecret,
		TokenURL:     microsoft.AzureADEndpoint(arg.TenantID).TokenURL,
		Scopes:       []string{azureMonitorScope},
	}

	// Reach the token endpoint through the same transport (proxy / CA /
	// skip_verify) as the data plane. We use a background context carrying the
	// http client so token refresh still works during the shutdown flush. The
	// client carries its own timeout because the background context has no
	// deadline and the data-plane client's timeout does not cover token
	// acquisition - without it a hung token endpoint would wedge the uploader.
	tokenCtx := context.WithValue(context.Background(), oauth2.HTTPClient,
		&http.Client{Transport: transport, Timeout: 30 * time.Second})

	return conf.TokenSource(tokenCtx), nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// azureMonitorBackoff computes exponential backoff for a retry attempt,
// capped at azureMonitorMaxRetryAfter. The cap also guards against int64
// overflow in retryWaitSecs*2^attempt when attempt or retry_wait is large.
func azureMonitorBackoff(retryWaitSecs, attempt int64) time.Duration {
	if attempt > 30 {
		attempt = 30
	}
	backoff := time.Duration(retryWaitSecs) * time.Second * time.Duration(int64(1)<<uint(attempt))
	if backoff <= 0 || backoff > azureMonitorMaxRetryAfter {
		return azureMonitorMaxRetryAfter
	}
	return backoff
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}

	var d time.Duration

	// Retry-After may be a number of seconds.
	if secs, err := strconv.Atoi(v); err == nil {
		if secs > 0 {
			d = time.Duration(secs) * time.Second
		}
	} else if t, err := http.ParseTime(v); err == nil {
		// Or an HTTP date.
		d = time.Until(t)
	}

	if d < 0 {
		return 0
	}
	if d > azureMonitorMaxRetryAfter {
		return azureMonitorMaxRetryAfter
	}
	return d
}

func responseOrError(body string, err error) string {
	if body != "" {
		return body
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func (self _AzureMonitorPlugin) maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope, arg *_AzureMonitorPluginArgs) error {

	// Not running on the server, secrets don't work.
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

	// If an explicit secret is defined let it provide the credentials.
	if arg.Secret != "" {
		return nil
	}

	return utils.SecretsEnforced
}

func mergeSecretAzureMonitor(
	ctx context.Context, scope vfilter.Scope, arg *_AzureMonitorPluginArgs) error {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return errors.New("azure_monitor_upload: Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	s, err := secrets_service.GetSecret(ctx, principal,
		constants.AZURE_MONITOR_CREDS, arg.Secret)
	if err != nil {
		return err
	}

	// Allow the user to override these fields.
	s.UpdateString("logs_ingestion_endpoint", &arg.LogsIngestionEndpoint)
	s.UpdateString("dcr_immutable_id", &arg.DCRImmutableID)
	s.UpdateString("stream_name", &arg.StreamName)
	s.UpdateString("table", &arg.Table)

	// Credentials come from the secret when it carries them. Unlike ADX_CREDS
	// (whose verifier requires all three, so mergeSecretADX can assign
	// unconditionally), the AZURE_MONITOR_CREDS verifier makes credentials
	// optional - auth may instead be a managed identity, the default
	// credential chain or the AZURE_* env vars. So a destination-only secret
	// must leave any credentials the caller supplied alone rather than
	// blanking them. A secret that does carry credentials still wins.
	s.UpdateString("client_id", &arg.ClientID)
	s.UpdateString("client_secret", &arg.ClientSecret)
	s.UpdateString("tenant_id", &arg.TenantID)
	s.UpdateBool("managed_identity", &arg.ManagedIdentity)
	s.UpdateString("managed_identity_client_id", &arg.ManagedIdentityClient)
	s.UpdateBool("default_credential", &arg.DefaultCredential)
	s.UpdateString("root_ca", &arg.RootCerts)
	s.UpdateBool("skip_verify", &arg.SkipVerify)

	return nil
}

func (self _AzureMonitorPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "azure_monitor_upload",
		Doc:      "Upload rows to Azure Monitor / Log Analytics via the Logs Ingestion API (Data Collection Rule).",
		ArgType:  type_map.AddType(scope, &_AzureMonitorPluginArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.NETWORK).Build(),
		Version:  1,
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_AzureMonitorPlugin{})
}
