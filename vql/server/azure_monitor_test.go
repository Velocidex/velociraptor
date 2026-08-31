//go:build sumo
// +build sumo

package server

import (
	"context"
	stdjson "encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

func TestBuildIngestionURL(t *testing.T) {
	const want = "https://x.region.ingest.monitor.azure.com/dataCollectionRules/dcr-abc123/streams/Custom-Foo_CL?api-version=2023-01-01"

	cases := []struct {
		name, endpoint, dcr, stream string
	}{
		{"plain", "https://x.region.ingest.monitor.azure.com", "dcr-abc123", "Custom-Foo_CL"},
		{"trailing slash trimmed", "https://x.region.ingest.monitor.azure.com/", "dcr-abc123", "Custom-Foo_CL"},
	}

	for _, c := range cases {
		got := buildIngestionURL(c.endpoint, c.dcr, c.stream)
		if got != want {
			t.Errorf("%s: buildIngestionURL(%q,%q,%q) = %q, want %q",
				c.name, c.endpoint, c.dcr, c.stream, got, want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"5", 5 * time.Second},
		{" 5 ", 5 * time.Second},
		{"", 0},
		{"0", 0},
		{"-3", 0},
		{"garbage", 0},
	}
	for _, c := range cases {
		if got := parseRetryAfter(c.in); got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMarshalAzureRow(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()
	ctx := context.Background()
	opts := vql_subsystem.EncOptsFromScope(scope)

	row := ordereddict.NewDict().
		Set("_Artifact", "Windows.System.Pslist").
		Set("_ClientId", "C.1234").
		Set("_FlowId", "F.5678").
		Set("_Organization", "root").
		Set("_Hostname", "HOST-1").
		Set("_timestamp", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)).
		Set("Pid", 4321).
		Set("Name", "evil.exe")

	data, err := marshal_azure_row(ctx, scope, row, opts)
	if err != nil {
		t.Fatalf("marshal_azure_row: %v", err)
	}

	var out map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(data, &out); err != nil {
		t.Fatalf("output is not valid JSON: %v (%s)", err, data)
	}

	// Structured metadata columns are lifted out of the raw row.
	assertJSONString(t, out, "Artifact", "Windows.System.Pslist")
	assertJSONString(t, out, "ClientId", "C.1234")
	assertJSONString(t, out, "FlowId", "F.5678")
	assertJSONString(t, out, "Organization", "root")
	assertJSONString(t, out, "Hostname", "HOST-1")

	// TimeGenerated is mandatory for every Log Analytics table.
	var tg string
	_ = stdjson.Unmarshal(out["TimeGenerated"], &tg)
	if tg == "" {
		t.Errorf("TimeGenerated missing/empty: %s", data)
	}

	// RawData holds the original row minus the injected metadata fields.
	raw := map[string]stdjson.RawMessage{}
	if err := stdjson.Unmarshal(out["RawData"], &raw); err != nil {
		t.Fatalf("RawData is not an object: %v", err)
	}
	for _, k := range []string{"_Artifact", "_ClientId", "_FlowId", "_Organization", "_Hostname", "_timestamp"} {
		if _, pres := raw[k]; pres {
			t.Errorf("RawData should not contain injected metadata %q: %s", k, out["RawData"])
		}
	}
	if _, pres := raw["Name"]; !pres {
		t.Errorf("RawData should contain the original Name column: %s", out["RawData"])
	}

	// The row has no Timestamp column, so there is no event time to report.
	if _, pres := out["EventTime"]; pres {
		t.Errorf("EventTime should be omitted when the row has no Timestamp: %s", data)
	}
}

// TimeGenerated must be the ingestion time (Azure only accepts a narrow window
// around it), while the artifact's own event time goes to EventTime.
func TestMarshalAzureRowSplitsEventTime(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()
	ctx := context.Background()
	opts := vql_subsystem.EncOptsFromScope(scope)

	row := ordereddict.NewDict().
		Set("Timestamp", time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC)).
		Set("_timestamp", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))

	data, err := marshal_azure_row(ctx, scope, row, opts)
	if err != nil {
		t.Fatalf("marshal_azure_row: %v", err)
	}

	var out map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(data, &out); err != nil {
		t.Fatalf("output is not valid JSON: %v (%s)", err, data)
	}

	var tg string
	_ = stdjson.Unmarshal(out["TimeGenerated"], &tg)
	if len(tg) < 4 || tg[:4] != "2024" {
		t.Errorf("TimeGenerated = %q, want it derived from _timestamp (2024...)", tg)
	}

	var et string
	_ = stdjson.Unmarshal(out["EventTime"], &et)
	if len(et) < 4 || et[:4] != "2021" {
		t.Errorf("EventTime = %q, want it derived from the Timestamp column (2021...)", et)
	}

	// The event time also stays in RawData, as the artifact emitted it.
	raw := map[string]stdjson.RawMessage{}
	if err := stdjson.Unmarshal(out["RawData"], &raw); err != nil {
		t.Fatalf("RawData is not an object: %v", err)
	}
	if _, pres := raw["Timestamp"]; !pres {
		t.Errorf("RawData should retain the original Timestamp column: %s", out["RawData"])
	}
}

// TimeFromAny returns the zero time with a nil error for several inputs.
// formatAzureTime must report those as "" so marshal_azure_row falls back
// rather than emitting a year-0001 TimeGenerated that Azure drops.
func TestFormatAzureTimeZeroValues(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()
	ctx := context.Background()

	cases := []struct {
		name string
		in   interface{}
	}{
		{"empty string", ""},
		{"zero epoch", 0},
		// The timestamp LRU caches failed parses too, so a repeated
		// unparseable string comes back as a hit with a nil error.
		{"unparseable string", "not a timestamp"},
		{"unparseable string repeated", "not a timestamp"},
	}

	for _, c := range cases {
		if got := formatAzureTime(ctx, scope, c.in); got != "" {
			t.Errorf("%s: formatAzureTime(%#v) = %q, want \"\"", c.name, c.in, got)
		}
	}
}

// An empty Timestamp column yields no EventTime at all, and leaves the
// injected _timestamp as TimeGenerated.
func TestMarshalAzureRowEmptyTimestampHasNoEventTime(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()
	ctx := context.Background()
	opts := vql_subsystem.EncOptsFromScope(scope)

	row := ordereddict.NewDict().
		Set("Timestamp", "").
		Set("_timestamp", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))

	out := marshalAzureRowForTest(t, ctx, scope, row, opts)

	var tg string
	_ = stdjson.Unmarshal(out["TimeGenerated"], &tg)
	if len(tg) < 4 || tg[:4] != "2024" {
		t.Errorf("TimeGenerated = %q, want it derived from _timestamp (2024...)", tg)
	}
	if _, pres := out["EventTime"]; pres {
		t.Errorf("EventTime should be omitted for an empty Timestamp: %s", out["EventTime"])
	}
}

// Without a _timestamp, TimeGenerated falls all the way back to now - never to
// the zero time - and an unparseable Timestamp produces no EventTime.
func TestMarshalAzureRowUnparseableTimestampFallsBackToNow(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()
	ctx := context.Background()
	opts := vql_subsystem.EncOptsFromScope(scope)

	row := ordereddict.NewDict().
		Set("Timestamp", "not a timestamp").
		Set("Name", "evil.exe")

	// Marshal twice: the second row exercises the cached-failed-parse path.
	for i := 0; i < 2; i++ {
		out := marshalAzureRowForTest(t, ctx, scope, row, opts)

		var tg string
		if err := stdjson.Unmarshal(out["TimeGenerated"], &tg); err != nil {
			t.Fatalf("row %d: TimeGenerated is not a JSON string: %v", i, err)
		}
		parsed, err := time.Parse(time.RFC3339Nano, tg)
		if err != nil {
			t.Fatalf("row %d: TimeGenerated %q is not RFC3339: %v", i, tg, err)
		}
		if parsed.Year() < 2024 {
			t.Errorf("row %d: TimeGenerated = %q, want the time.Now() fallback", i, tg)
		}
		if _, pres := out["EventTime"]; pres {
			t.Errorf("row %d: EventTime should be omitted for an unparseable Timestamp: %s",
				i, out["EventTime"])
		}
	}
}

// An error response body must not be relayed to the logs and the result row in
// its entirety - a broken endpoint can return an arbitrarily large error page.
func TestPostAzureBatchTruncatesResponse(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	huge := strings.Repeat("A", 1024*1024)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(huge))
		}))
	defer server.Close()

	arg := &_AzureMonitorPluginArgs{MaxRetries: 0, RetryWait: 1}
	status, body, err := post_azure_batch(context.Background(), scope,
		server.Client(), server.URL, []byte("ignored"), arg)

	if err == nil {
		t.Fatalf("expected an error for a 400 response")
	}
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if len(body) > azureMonitorMaxRespBody+64 {
		t.Errorf("response body is %d bytes, want it capped near %d",
			len(body), azureMonitorMaxRespBody)
	}
	if !strings.HasSuffix(body, "...(truncated)") {
		t.Errorf("a capped body should be marked truncated, got %d bytes ending %q",
			len(body), body[max(0, len(body)-20):])
	}
}

func marshalAzureRowForTest(
	t *testing.T, ctx context.Context, scope vfilter.Scope,
	row *ordereddict.Dict, opts *json.EncOpts) map[string]stdjson.RawMessage {
	t.Helper()

	data, err := marshal_azure_row(ctx, scope, row, opts)
	if err != nil {
		t.Fatalf("marshal_azure_row: %v", err)
	}

	var out map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(data, &out); err != nil {
		t.Fatalf("output is not valid JSON: %v (%s)", err, data)
	}
	return out
}

func assertJSONString(t *testing.T, m map[string]stdjson.RawMessage, key, want string) {
	t.Helper()
	raw, pres := m[key]
	if !pres {
		t.Errorf("missing key %q", key)
		return
	}
	var got string
	if err := stdjson.Unmarshal(raw, &got); err != nil {
		t.Errorf("key %q is not a JSON string: %v", key, err)
		return
	}
	if got != want {
		t.Errorf("key %q = %q, want %q", key, got, want)
	}
}
