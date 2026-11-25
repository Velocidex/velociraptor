package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"www.velocidex.com/golang/vfilter"
)

func consumeScope(brokers []string) vfilter.Scope {
	return newScope(brokers).AppendPlugins(KafkaConsumePlugin{})
}

// produce writes records to a topic and waits until they are stored.
func produce(t *testing.T, brokers []string, records ...*kgo.Record) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cl.ProduceSync(ctx, records...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

func jsonRecord(topic, key, value string) *kgo.Record {
	r := &kgo.Record{Topic: topic, Value: []byte(value)}
	if key != "" {
		r.Key = []byte(key)
	}
	return r
}

// A JSON object's fields become columns, numbers stay numbers, nested
// objects stay objects, and the record's topic, key, headers and time
// arrive in underscore columns.
func TestConsumeReadsJSON(t *testing.T) {
	c := newCluster(t, "alerts")
	r := jsonRecord("alerts", "C.1", `{"Host":"web1","Severity":7,"Score":0.5,"Rule":{"Name":"x"}}`)
	r.Headers = []kgo.RecordHeader{{Key: "Source", Value: []byte("siem")}}
	produce(t, c.ListenAddrs(), r)

	rows := runVQL(context.Background(), t, consumeScope(c.ListenAddrs()), `
SELECT * FROM kafka_consume(brokers=Brokers, topics="alerts",
  start="earliest", max_records=1)`)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	for _, want := range []struct {
		col   string
		value interface{}
	}{
		{"Host", "web1"}, {"Severity", uint64(7)}, {"Score", 0.5},
		{"_Topic", "alerts"}, {"_Key", "C.1"}, {"_Offset", int64(0)},
	} {
		got, _ := row.Get(want.col)
		if got != want.value {
			t.Errorf("%s = %#v, want %#v", want.col, got, want.value)
		}
	}
	rule, _ := row.Get("Rule")
	if d, ok := rule.(*ordereddict.Dict); !ok {
		t.Errorf("Rule = %#v, want a dict", rule)
	} else if name, _ := d.Get("Name"); name != "x" {
		t.Errorf("Rule.Name = %#v", name)
	}
	headers, _ := row.Get("_Headers")
	if d, _ := headers.(*ordereddict.Dict); d == nil {
		t.Errorf("_Headers = %#v", headers)
	} else if v, _ := d.Get("Source"); v != "siem" {
		t.Errorf("_Headers.Source = %#v", v)
	}
	if ts, _ := row.Get("_Time"); ts == nil {
		t.Error("no _Time")
	}
}

// decode=raw keeps the value as text; with decode=json, a value that is
// not a JSON object is kept as text too rather than dropped.
func TestConsumeRawAndNonJSON(t *testing.T) {
	c := newCluster(t, "logs")
	produce(t, c.ListenAddrs(),
		jsonRecord("logs", "", `{"A":1}`),
		jsonRecord("logs", "", `plain text line`),
		jsonRecord("logs", "", `[1,2,3]`))

	scope := consumeScope(c.ListenAddrs())
	json_rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_consume(brokers=Brokers, topics="logs", start="earliest", max_records=3)`)
	raw_rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_consume(brokers=Brokers, topics="logs", start="earliest", max_records=3, decode="raw")`)
	if len(json_rows) != 3 || len(raw_rows) != 3 {
		t.Fatalf("got %d json rows and %d raw rows, want 3 each", len(json_rows), len(raw_rows))
	}
	// Partitions are read in no fixed order, so look rows up by value.
	values := map[string]bool{}
	for _, r := range json_rows {
		if a, ok := r.Get("A"); ok {
			if a != uint64(1) {
				t.Errorf("A = %#v", a)
			}
			continue
		}
		v, _ := r.Get("Value")
		values[fmt.Sprint(v)] = true
	}
	if !values["plain text line"] || !values["[1,2,3]"] {
		t.Errorf("non-object values not kept as text: %v", values)
	}
	for _, r := range raw_rows {
		if _, ok := r.Get("A"); ok {
			t.Error("decode=raw decoded JSON")
		}
		if v, _ := r.Get("Value"); v == nil {
			t.Error("decode=raw row without Value")
		}
	}
}

// With a group, records are committed once they are rows: a second run
// carries on after the first, with nothing read twice or skipped.
// Without a group, every run starts over.
func TestConsumeGroupResumes(t *testing.T) {
	c := newCluster(t, "tasks")
	var records []*kgo.Record
	for i := 0; i < 6; i++ {
		records = append(records, jsonRecord("tasks", "same-key", fmt.Sprintf(`{"N":%d}`, i)))
	}
	produce(t, c.ListenAddrs(), records...)
	scope := consumeScope(c.ListenAddrs())

	logs := &logBuffer{}
	scope.SetLogger(log.New(logs, "", 0))
	var timings []string
	read := func(q string) []uint64 {
		start := time.Now()
		var ns []uint64
		for _, r := range runVQL(context.Background(), t, scope, q) {
			n, _ := r.Get("N")
			ns = append(ns, n.(uint64))
		}
		timings = append(timings, fmt.Sprintf("%v rows in %v", len(ns), time.Since(start).Round(10*time.Millisecond)))
		return ns
	}

	first := read(`SELECT * FROM kafka_consume(brokers=Brokers, topics="tasks",
  group="g1", start="earliest", max_records=2)`)
	rest := read(`SELECT * FROM kafka_consume(brokers=Brokers, topics="tasks",
  group="g1", start="earliest", idle_timeout=3)`)
	if fmt.Sprint(first) != "[0 1]" || fmt.Sprint(rest) != "[2 3 4 5]" {
		t.Fatalf("group runs read %v then %v, want [0 1] then [2 3 4 5]\nruns: %v\nlog:\n%s",
			first, rest, timings, logs)
	}

	again := read(`SELECT * FROM kafka_consume(brokers=Brokers, topics="tasks",
  start="earliest", max_records=2)`)
	if fmt.Sprint(again) != "[0 1]" {
		t.Fatalf("without a group read %v, want [0 1] (from the start)", again)
	}
}

// start="latest" (the default) skips what was already in the topic.
func TestConsumeLatestSkipsOld(t *testing.T) {
	c := newCluster(t, "events")
	produce(t, c.ListenAddrs(), jsonRecord("events", "k", `{"Old":true}`))

	go func() {
		// No t.Fatal here: it must not be called outside the test's
		// own goroutine.
		time.Sleep(1500 * time.Millisecond)
		cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...))
		if err != nil {
			t.Error(err)
			return
		}
		defer cl.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := cl.ProduceSync(ctx, jsonRecord("events", "k", `{"New":true}`)).FirstErr(); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows := runVQL(ctx, t, consumeScope(c.ListenAddrs()), `
SELECT * FROM kafka_consume(brokers=Brokers, topics="events", max_records=1)`)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if _, ok := rows[0].Get("New"); !ok {
		t.Fatalf("read %v, want only the record produced after starting", rows[0])
	}
}

// idle_timeout ends a query on a quiet topic.
func TestConsumeIdleTimeout(t *testing.T) {
	c := newCluster(t, "quiet")
	start := time.Now()
	rows := runVQL(context.Background(), t, consumeScope(c.ListenAddrs()), `
SELECT * FROM kafka_consume(brokers=Brokers, topics="quiet", idle_timeout=1)`)
	took := time.Since(start)
	if len(rows) != 0 || took > 5*time.Second {
		t.Fatalf("got %d rows after %v, want none within a few seconds", len(rows), took)
	}
}

// Topics starting with ^ are patterns.
func TestConsumeTopicPattern(t *testing.T) {
	c := newCluster(t, "vr.a", "vr.b", "other")
	produce(t, c.ListenAddrs(),
		jsonRecord("vr.a", "", `{"From":"a"}`),
		jsonRecord("vr.b", "", `{"From":"b"}`),
		jsonRecord("other", "", `{"From":"other"}`))
	rows := runVQL(context.Background(), t, consumeScope(c.ListenAddrs()), `
SELECT * FROM kafka_consume(brokers=Brokers, topics="^vr[.].*",
  start="earliest", idle_timeout=3)`)
	seen := map[string]bool{}
	for _, r := range rows {
		topic, _ := r.Get("_Topic")
		seen[topic.(string)] = true
	}
	if len(rows) != 2 || !seen["vr.a"] || !seen["vr.b"] {
		t.Fatalf("read topics %v (%d rows), want vr.a and vr.b only", seen, len(rows))
	}
}

// Arguments that cannot work are refused with a reason, and no rows.
func TestConsumeBadArgs(t *testing.T) {
	c := newCluster(t, "t")
	for _, tc := range []struct{ args, logHas string }{
		{`brokers=Brokers, topics="t", decode="xml"`, "decode must be json or raw"},
		{`brokers=Brokers, topics="t", start="middle"`, "start must be latest or earliest"},
		{`brokers=Brokers, topics=["^t.*", "plain"]`, "either all names are regular expressions"},
		{`topics="t"`, "no brokers"},
		{`brokers=Brokers, topics="t", secret="x"`, "secret"},
	} {
		logs := &logBuffer{}
		scope := consumeScope(c.ListenAddrs())
		scope.SetLogger(log.New(logs, "", 0))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		rows := runVQL(ctx, t, scope, `SELECT * FROM kafka_consume(`+tc.args+`)`)
		cancel()
		if len(rows) != 0 || !strings.Contains(logs.String(), tc.logHas) {
			t.Errorf("%s: %d rows, log %q; want no rows and %q", tc.args, len(rows), logs, tc.logHas)
		}
	}
}

// A consumed row passed straight to kafka_upload() is not routed by its
// _Topic column: only a lowercase _topic column routes.
func TestConsumeIntoUpload(t *testing.T) {
	c := newCluster(t, "in", "out")
	produce(t, c.ListenAddrs(), jsonRecord("in", "k", `{"N":1}`), jsonRecord("in", "k", `{"N":2}`))
	rows := runVQL(context.Background(), t, consumeScope(c.ListenAddrs()), `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM kafka_consume(brokers=Brokers, topics="in",
            start="earliest", max_records=2) },
  brokers=Brokers, topic="out")`)
	if sent, failed := totals(rows); sent != 2 || failed != 0 {
		t.Fatalf("sent=%d failed=%d, want 2/0", sent, failed)
	}
	if got := consume(t, c.ListenAddrs(), "out", 2, 5*time.Second); len(got) != 2 {
		t.Fatalf("out has %d records, want 2", len(got))
	}
}

// The connection options are the same as kafka_upload()'s: here SCRAM
// over TLS.
func TestConsumeSCRAMOverTLS(t *testing.T) {
	pki := newTestPKI(t)
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "secure"),
		kfake.TLS(pki.Server), kfake.EnableSASL(),
		kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(pki.CAPEM))
	tls_cfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	mech, _ := parseSASL("SCRAM-SHA-256", "vr", "s3cret")
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.DialTLSConfig(tls_cfg), kgo.SASL(mech))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = cl.ProduceSync(ctx, jsonRecord("secure", "", `{"Ok":true}`)).FirstErr()
	cancel()
	cl.Close()
	if err != nil {
		t.Fatal(err)
	}

	scope := consumeScope(c.ListenAddrs()).AppendVars(ordereddict.NewDict().Set("CA", pki.CAPEM))
	rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_consume(brokers=Brokers, topics="secure", start="earliest",
  max_records=1, use_tls=TRUE, root_ca=CA,
  sasl_mechanism="SCRAM-SHA-256", username="vr", password="s3cret")`)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
}

// When nothing arrives because of a problem, the stall warning names it.
func TestConsumeStallWarningNamesTheCause(t *testing.T) {
	old := ConsumePollInterval
	ConsumePollInterval = 500 * time.Millisecond
	defer func() { ConsumePollInterval = old }()

	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "t"),
		kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	logs := &logBuffer{}
	scope := consumeScope(c.ListenAddrs())
	scope.SetLogger(log.New(logs, "", 0))
	runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_consume(brokers=Brokers, topics="t", idle_timeout=4,
  stall_warning=1, sasl_mechanism="SCRAM-SHA-256", username="vr", password="WRONG")`)
	if !strings.Contains(strings.ToLower(logs.String()), "sasl") {
		t.Fatalf("no stall warning naming the SASL failure:\n%s", logs)
	}
}

// A short idle_timeout with a group still reads: the wait to join the
// group (3s by default on a new group) does not count as idle.
func TestConsumeIdleCountsFromJoin(t *testing.T) {
	c := newCluster(t, "jobs")
	produce(t, c.ListenAddrs(), jsonRecord("jobs", "k", `{"N":1}`))
	rows := runVQL(context.Background(), t, consumeScope(c.ListenAddrs()), `
SELECT * FROM kafka_consume(brokers=Brokers, topics="jobs",
  group="fresh", start="earliest", idle_timeout=1)`)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: idle_timeout expired while joining the group", len(rows))
	}
}

// A group that cannot be joined (no broker) still ends the query once
// the join grace and idle_timeout have passed.
func TestConsumeJoinGraceIsBounded(t *testing.T) {
	old := GroupJoinGrace
	GroupJoinGrace = 2 * time.Second
	defer func() { GroupJoinGrace = old }()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runVQL(ctx, t, consumeScope([]string{"127.0.0.1:1"}), `
SELECT * FROM kafka_consume(brokers=Brokers, topics="t", group="g", idle_timeout=1)`)
	if took := time.Since(start); took > 8*time.Second {
		t.Fatalf("returned after %v, want shortly after the 2s join grace", took)
	}
}

// When idle_timeout ends a query that read nothing because of a problem,
// the log says what the problem was, even before any stall warning.
func TestConsumeIdleStopNamesTheCause(t *testing.T) {
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "t"),
		kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	logs := &logBuffer{}
	scope := consumeScope(c.ListenAddrs())
	scope.SetLogger(log.New(logs, "", 0))
	runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_consume(brokers=Brokers, topics="t", idle_timeout=2,
  sasl_mechanism="SCRAM-SHA-256", username="vr", password="WRONG")`)
	if !strings.Contains(logs.String(), "stopping after") ||
		!strings.Contains(strings.ToLower(logs.String()), "sasl") {
		t.Fatalf("idle stop does not name the SASL failure:\n%s", logs)
	}
}
