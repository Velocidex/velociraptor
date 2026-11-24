package kafka

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// test_rows(count, hosts, routed, hold) emits fake event rows. With
// hold=TRUE it keeps the query open after the last row, like a
// long-running event artifact, so only the wait_time timer can flush.
type testRowsArgs struct {
	Count  int64 `vfilter:"required,field=count"`
	Hosts  int64 `vfilter:"optional,field=hosts"`
	Routed bool  `vfilter:"optional,field=routed"`
	Hold   bool  `vfilter:"optional,field=hold"`
}

type testRows struct{}

func (self testRows) Info(scope vfilter.Scope, tm *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{Name: "test_rows"}
}

func (self testRows) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	out := make(chan vfilter.Row)
	go func() {
		defer close(out)
		a := &testRowsArgs{}
		if err := arg_parser.ExtractArgsWithContext(ctx, scope, args, a); err != nil {
			scope.Log("test_rows: %v", err)
			return
		}
		if a.Hosts <= 0 {
			a.Hosts = 1
		}
		for i := int64(0); i < a.Count; i++ {
			row := ordereddict.NewDict().
				Set("N", i).
				Set("ClientId", fmt.Sprintf("C.%d", i%a.Hosts)).
				Set("Image", `C:\Windows\System32\cmd.exe`)
			if a.Routed {
				row.Set("_topic", []string{"vr.process", "vr.network"}[i%2])
			}
			select {
			case <-ctx.Done():
				return
			case out <- row:
			}
		}
		if a.Hold {
			<-ctx.Done()
		}
	}()
	return out
}

func newCluster(t *testing.T, topics ...string) *kfake.Cluster {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(3, topics...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func newScope(brokers []string) vfilter.Scope {
	return vfilter.NewScope().
		AppendPlugins(KafkaUploadPlugin{}, testRows{}).
		AppendVars(ordereddict.NewDict().Set("Brokers", brokers).
			Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
}

func runVQL(ctx context.Context, t *testing.T, scope vfilter.Scope, q string) []*ordereddict.Dict {
	t.Helper()
	vql, err := vfilter.Parse(q)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var rows []*ordereddict.Dict
	for row := range vql.Eval(ctx, scope) {
		rows = append(rows, row.(*ordereddict.Dict))
	}
	return rows
}

func totals(rows []*ordereddict.Dict) (sent, failed int) {
	for _, r := range rows {
		s, _ := r.Get("Sent")
		f, _ := r.Get("Failed")
		sent += s.(int)
		failed += f.(int)
	}
	return
}

// Read back n records from a topic, giving up after the deadline.
func consume(t *testing.T, brokers []string, topic string, n int, within time.Duration) []*kgo.Record {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	var got []*kgo.Record
	for len(got) < n {
		fetches := cl.PollFetches(ctx)
		if ctx.Err() != nil {
			break
		}
		fetches.EachRecord(func(r *kgo.Record) { got = append(got, r) })
	}
	return got
}

// 1000 rows from 3 hosts, batched in chunks of 100. Every row arrives,
// keyed by ClientId, with headers attached and per-host order kept.
func TestUploadAndOrdering(t *testing.T) {
	c := newCluster(t, "vr.events")
	brokers := c.ListenAddrs()
	scope := newScope(brokers)

	rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=1000, hosts=3) },
  brokers=Brokers, topic="vr.events", key_field="ClientId",
  chunk_size=100, wait_time=5,
  headers=dict(Artifact="Windows.Detection.ProcessCreation", Org="root"),
  header_fields=["ClientId"])`)

	sent, failed := totals(rows)
	if sent != 1000 || failed != 0 {
		t.Fatalf("sent=%d failed=%d, want 1000/0", sent, failed)
	}
	if len(rows) != 10 {
		t.Errorf("got %d batch result rows, want 10", len(rows))
	}

	recs := consume(t, brokers, "vr.events", 1000, 10*time.Second)
	if len(recs) != 1000 {
		t.Fatalf("consumed %d records, want 1000", len(recs))
	}

	last := map[string]int64{}
	partitionOf := map[string]int32{}
	for _, r := range recs {
		var ev struct {
			N        int64
			ClientId string
			Image    string
		}
		if err := json.Unmarshal(r.Value, &ev); err != nil {
			t.Fatalf("bad JSON %q: %v", r.Value, err)
		}
		if string(r.Key) != ev.ClientId {
			t.Fatalf("key %q != ClientId %q", r.Key, ev.ClientId)
		}
		if ev.Image != `C:\Windows\System32\cmd.exe` {
			t.Fatalf("value mangled: %q", ev.Image)
		}
		// Same key always lands on the same partition.
		if p, ok := partitionOf[ev.ClientId]; ok && p != r.Partition {
			t.Fatalf("%s split across partitions %d and %d", ev.ClientId, p, r.Partition)
		}
		partitionOf[ev.ClientId] = r.Partition
		// And arrives in order within that host.
		if prev, ok := last[ev.ClientId]; ok && ev.N <= prev {
			t.Fatalf("%s out of order: %d after %d", ev.ClientId, ev.N, prev)
		}
		last[ev.ClientId] = ev.N

		// Two static headers, then ClientId promoted from the row.
		if len(r.Headers) != 3 || r.Headers[0].Key != "Artifact" ||
			string(r.Headers[0].Value) != "Windows.Detection.ProcessCreation" ||
			r.Headers[2].Key != "ClientId" || string(r.Headers[2].Value) != ev.ClientId {
			t.Fatalf("headers wrong: %+v", r.Headers)
		}
	}
	var slowest int64
	for _, r := range rows {
		if ms, _ := r.Get("DurationMs"); ms.(int64) > slowest {
			slowest = ms.(int64)
		}
	}
	t.Logf("1000 records, %d hosts, partitions %v, slowest batch %dms",
		len(last), partitionOf, slowest)
}

// A long-running event query that never fills a chunk must still be
// flushed by the wait_time timer, while the query is still open.
func TestWaitTimeFlush(t *testing.T) {
	c := newCluster(t, "vr.events")
	brokers := c.ListenAddrs()
	scope := newScope(brokers)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan []*ordereddict.Dict)
	go func() {
		done <- runVQL(ctx, t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=5, hold=TRUE) },
  brokers=Brokers, topic="vr.events", chunk_size=1000, wait_time=1)`)
	}()

	start := time.Now()
	recs := consume(t, brokers, "vr.events", 5, 5*time.Second)
	elapsed := time.Since(start)
	cancel()
	<-done

	if len(recs) != 5 {
		t.Fatalf("timer flush delivered %d records, want 5", len(recs))
	}
	t.Logf("5 records flushed by timer after %v with the query still open", elapsed.Round(10*time.Millisecond))
}

// Without topic=, rows route by their _topic column, which is stripped
// from the payload.
func TestTopicRouting(t *testing.T) {
	c := newCluster(t, "vr.process", "vr.network")
	brokers := c.ListenAddrs()
	scope := newScope(brokers)

	rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=100, routed=TRUE) },
  brokers=Brokers, chunk_size=1000)`)
	if sent, failed := totals(rows); sent != 100 || failed != 0 {
		t.Fatalf("sent=%d failed=%d", sent, failed)
	}

	for _, topic := range []string{"vr.process", "vr.network"} {
		recs := consume(t, brokers, topic, 50, 5*time.Second)
		if len(recs) != 50 {
			t.Fatalf("%s got %d records, want 50", topic, len(recs))
		}
		var m map[string]any
		json.Unmarshal(recs[0].Value, &m)
		if _, pres := m["_topic"]; pres {
			t.Fatalf("_topic leaked into payload: %s", recs[0].Value)
		}
	}
}

// With no broker listening, the plugin must give up within its timeout
// and report the failure in its result row, not hang the query.
func TestBrokerDown(t *testing.T) {
	scope := newScope([]string{"127.0.0.1:1"})

	start := time.Now()
	rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=10) },
  brokers=Brokers, topic="vr.events", timeout=2)`)
	elapsed := time.Since(start)

	sent, failed := totals(rows)
	if sent != 0 || failed != 10 {
		t.Fatalf("sent=%d failed=%d, want 0/10", sent, failed)
	}
	errStr, _ := rows[0].Get("Error")
	if errStr == "" {
		t.Fatal("expected an error message")
	}
	if elapsed > 10*time.Second {
		t.Fatalf("took %v to give up", elapsed)
	}
	t.Logf("gave up after %v: %v", elapsed.Round(10*time.Millisecond), errStr)
}

// SCRAM-SHA-256 against a broker that requires SASL: the right password
// delivers, the wrong one fails cleanly within the timeout.
func TestSASL(t *testing.T) {
	c, err := kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"),
		kfake.EnableSASL(),
		kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	brokers := c.ListenAddrs()

	for _, tc := range []struct {
		pass         string
		sent, failed int
	}{{"s3cret", 10, 0}, {"wrong", 0, 10}} {
		scope := newScope(brokers).AppendVars(ordereddict.NewDict().Set("Pass", tc.pass))
		rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=10) },
  brokers=Brokers, topic="vr.events", timeout=3,
  sasl_mechanism="SCRAM-SHA-256", username="vr", password=Pass)`)
		sent, failed := totals(rows)
		if sent != tc.sent || failed != tc.failed {
			t.Fatalf("password %q: sent=%d failed=%d, want %d/%d",
				tc.pass, sent, failed, tc.sent, tc.failed)
		}
	}
}

// Cancelling a running event query (stopping the monitoring artifact)
// must still deliver rows that were buffered, and return promptly.
func TestCancelFlushesBuffer(t *testing.T) {
	c := newCluster(t, "vr.events")
	brokers := c.ListenAddrs()
	scope := newScope(brokers)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runVQL(ctx, t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=7, hold=TRUE) },
  brokers=Brokers, topic="vr.events", chunk_size=1000, wait_time=60)`)
		close(done)
	}()

	time.Sleep(300 * time.Millisecond) // rows are buffered, no flush due yet
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("plugin did not return after cancel")
	}
	returned := time.Since(start)

	recs := consume(t, brokers, "vr.events", 7, 5*time.Second)
	if len(recs) != 7 {
		t.Fatalf("cancel delivered %d records, want 7", len(recs))
	}
	t.Logf("cancel flushed 7 buffered records and returned in %v", returned.Round(time.Millisecond))
}

func TestBadArgs(t *testing.T) {
	for _, q := range []string{
		`SELECT * FROM kafka_upload(query={SELECT 1 AS A FROM scope()}, brokers=["x:1"], topic="t", compression="brotli")`,
		`SELECT * FROM kafka_upload(query={SELECT 1 AS A FROM scope()}, brokers=["x:1"], topic="t", sasl_mechanism="GSSAPI")`,
	} {
		if rows := runVQL(context.Background(), t, newScope(nil), q); len(rows) != 0 {
			t.Fatalf("expected no rows for %s", q)
		}
	}
}

// A log sink tests can read while the plugin writes to it.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (self *logBuffer) Write(p []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.buf.Write(p)
}

func (self *logBuffer) String() string {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.buf.String()
}

// Make the broker accept produce requests but not answer them until
// release is closed: the same thing a broker going down mid-request
// looks like to the client.
func hangProduce(c *kfake.Cluster, release chan struct{}) {
	c.ControlKey(int16(kmsg.Produce), func(kmsg.Request) (kmsg.Response, error, bool) {
		c.KeepControl()
		c.SleepControl(func() { <-release })
		return nil, nil, false
	})
}

// hold=TRUE through an outage longer than timeout: nothing is failed,
// the stall warning fires while waiting, and every record arrives once
// the broker answers again.
func TestHoldThroughOutage(t *testing.T) {
	c := newCluster(t, "vr.events")
	brokers := c.ListenAddrs()
	release := make(chan struct{})
	hangProduce(c, release)

	logs := &logBuffer{}
	scope := newScope(brokers)
	scope.SetLogger(log.New(logs, "", 0))

	done := make(chan []*ordereddict.Dict)
	go func() {
		done <- runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=5) },
  brokers=Brokers, topic="vr.events",
  hold=TRUE, timeout=1, stall_warning=1)`)
	}()

	// Longer than timeout=1, so without hold the batch would fail.
	time.Sleep(3 * time.Second)
	close(release)

	var rows []*ordereddict.Dict
	select {
	case rows = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("hold never completed after the broker came back")
	}

	sent, failed := totals(rows)
	if sent != 5 || failed != 0 {
		t.Fatalf("sent=%d failed=%d, want 5/0\nlogs:\n%s", sent, failed, logs)
	}
	warnings := strings.Count(logs.String(), "waiting")
	if warnings < 2 {
		t.Fatalf("expected repeated stall warnings, got %d\nlogs:\n%s", warnings, logs)
	}
	if recs := consume(t, brokers, "vr.events", 5, 5*time.Second); len(recs) != 5 {
		t.Fatalf("consumed %d records, want 5", len(recs))
	}
	t.Logf("held through a 3s outage with timeout=1: %d stall warnings, 5/5 delivered", warnings)
}

// A free local port with nothing listening on it yet.
func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// The case hold mode exists for: the broker is unreachable before the
// batch is ever sent. Without hold, timeout applies and the batch is
// dropped; with hold, the records wait and are delivered once a broker
// appears on that address.
func TestHoldBrokerDownBeforeSend(t *testing.T) {
	for _, hold := range []bool{false, true} {
		port := freePort(t)
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		scope := newScope([]string{addr}).AppendVars(
			ordereddict.NewDict().Set("Hold", hold))

		done := make(chan []*ordereddict.Dict)
		go func() {
			done <- runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=5) },
  brokers=Brokers, topic="vr.events",
  hold=Hold, timeout=1, stall_warning=-1)`)
		}()

		// Down for longer than timeout=1, then the broker comes up.
		time.Sleep(3 * time.Second)
		c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.Ports(port),
			kfake.SeedTopics(1, "vr.events"))
		if err != nil {
			t.Fatal(err)
		}

		var rows []*ordereddict.Dict
		select {
		case rows = <-done:
		case <-time.After(15 * time.Second):
			c.Close()
			t.Fatalf("hold=%v: never finished", hold)
		}
		sent, failed := totals(rows)
		c.Close()

		switch {
		case hold && (sent != 5 || failed != 0):
			t.Fatalf("hold=TRUE: sent=%d failed=%d, want 5/0 (records must wait for the broker)", sent, failed)
		case !hold && (sent != 0 || failed != 5):
			t.Fatalf("hold=FALSE: sent=%d failed=%d, want 0/5 (timeout must drop them)", sent, failed)
		}
		t.Logf("hold=%v: broker down 3s with timeout=1 -> sent=%d failed=%d", hold, sent, failed)
	}
}

// Stopping a hold-mode upload while the broker is still hung must
// return after the shutdown grace instead of waiting forever. This
// calls the plugin directly: a VQL consumer stops reading as soon as
// it is cancelled, so only the plugin's own output channel closing
// shows that the plugin has really finished.
func TestHoldStopIsBounded(t *testing.T) {
	old := HoldShutdownGrace
	HoldShutdownGrace = time.Second
	defer func() { HoldShutdownGrace = old }()

	c := newCluster(t, "vr.events")
	release := make(chan struct{})
	defer close(release)
	hangProduce(c, release)

	query, err := vfilter.Parse("SELECT * FROM test_rows(count=3, hold=TRUE)")
	if err != nil {
		t.Fatal(err)
	}
	args := ordereddict.NewDict().
		Set("query", query).
		Set("brokers", c.ListenAddrs()).
		Set("topic", "vr.events").
		Set("hold", true).
		Set("wait_time", 1).
		Set("stall_warning", -1)

	ctx, cancel := context.WithCancel(context.Background())
	output := KafkaUploadPlugin{}.Call(ctx, newScope(nil), args)
	finished := make(chan struct{})
	go func() {
		for range output {
		}
		close(finished)
	}()

	time.Sleep(1500 * time.Millisecond) // the batch is now stuck at the broker
	start := time.Now()
	cancel()
	select {
	case <-finished:
	case <-time.After(6 * time.Second):
		t.Fatal("stopping a held upload hung on the dead broker")
	}
	took := time.Since(start)
	if took < HoldShutdownGrace/2 {
		t.Fatalf("returned after %v, before the grace period; did it really wait?", took)
	}
	t.Logf("stopped %v after cancel (grace %v)", took.Round(10*time.Millisecond), HoldShutdownGrace)
}

// Without hold, the old behavior stands: an unreachable broker fails
// the batch after timeout. (TestBrokerDown covers the details.)
func TestNoBrokersIsAnError(t *testing.T) {
	logs := &logBuffer{}
	scope := newScope(nil)
	scope.SetLogger(log.New(logs, "", 0))
	rows := runVQL(context.Background(), t, scope,
		`SELECT * FROM kafka_upload(query={SELECT 1 AS A FROM scope()}, topic="t")`)
	if len(rows) != 0 || !strings.Contains(logs.String(), "no brokers") {
		t.Fatalf("rows=%d logs=%q", len(rows), logs)
	}
}

func TestApplySecret(t *testing.T) {
	arg := &KafkaUploadArgs{
		Secret:   "prod",
		Brokers:  []string{"from-query:9092"},
		Username: "query-user",
		Topic:    "kept",
	}
	secret := ordereddict.NewDict().
		Set("brokers", "# primary cluster\nk1:9092, k2:9092\n\nk3:9092\n").
		Set("sasl_mechanism", "SCRAM-SHA-512").
		Set("username", "vr").
		Set("password", "s3cret").
		Set("use_tls", "Y").
		Set("skip_verify", "").
		Set("root_ca", "")

	if err := ApplySecret(arg, secret); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(arg.Brokers) != "[k1:9092 k2:9092 k3:9092]" {
		t.Errorf("brokers = %v", arg.Brokers)
	}
	if arg.SASLMechanism != "SCRAM-SHA-512" || arg.Username != "vr" || arg.Password != "s3cret" {
		t.Errorf("auth not taken from secret: %q %q %q", arg.SASLMechanism, arg.Username, arg.Password)
	}
	if !arg.UseTLS || arg.SkipVerify {
		t.Errorf("use_tls=%v skip_verify=%v, want true/false", arg.UseTLS, arg.SkipVerify)
	}
	if arg.Topic != "kept" {
		t.Errorf("non-secret field changed: topic=%q", arg.Topic)
	}

	err := ApplySecret(&KafkaUploadArgs{Secret: "empty"},
		ordereddict.NewDict().Set("brokers", "# nothing here\n"))
	if err == nil || !strings.Contains(err.Error(), "no brokers") {
		t.Errorf("secret without brokers: err=%v", err)
	}
}

// Credentials that exist only in the secret authenticate against a
// SCRAM-protected broker; the query passes no brokers or password.
func TestSecretDrivesSCRAM(t *testing.T) {
	c, err := kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"),
		kfake.EnableSASL(),
		kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	arg := &KafkaUploadArgs{Secret: "local", Topic: "vr.events", Timeout: 3}
	err = ApplySecret(arg, ordereddict.NewDict().
		Set("brokers", strings.Join(c.ListenAddrs(), "\n")).
		Set("sasl_mechanism", "SCRAM-SHA-256").
		Set("username", "vr").
		Set("password", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(arg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.ProduceSync(ctx, &kgo.Record{Value: []byte("hi")}).FirstErr(); err != nil {
		t.Fatalf("produce with secret credentials: %v", err)
	}
}

// A broker that requires SASL refuses a client that sends none.
func TestSASLRequiredWithoutCredentials(t *testing.T) {
	c, err := kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"),
		kfake.EnableSASL(),
		kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	rows := runVQL(context.Background(), t, newScope(c.ListenAddrs()), `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=10) },
  brokers=Brokers, topic="vr.events", timeout=2)`)
	if sent, failed := totals(rows); sent != 0 || failed != 10 {
		t.Fatalf("sent=%d failed=%d, want 0/10", sent, failed)
	}
	// The error should point at the missing SASL, not just a timeout.
	if e, _ := rows[0].Get("Error"); !strings.Contains(strings.ToLower(e.(string)), "sasl") {
		t.Fatalf("error does not mention SASL: %q", e)
	}
}

// The plugin batches rows itself, so the client must not add its own
// linger delay on top (franz-go waits 10ms by default).
func TestNoClientLinger(t *testing.T) {
	c := newCluster(t, "vr.events")
	client, err := NewClient(&KafkaUploadArgs{Brokers: c.ListenAddrs()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if linger := client.OptValue(kgo.ProducerLinger); linger != time.Duration(0) {
		t.Fatalf("producer linger = %v, want 0", linger)
	}
}

// A misconfiguration that hold mode waits on (bad credentials) must be
// named in the stall warning, so nobody goes looking at the broker.
func TestStallWarningNamesTheCause(t *testing.T) {
	c, err := kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"),
		kfake.EnableSASL(),
		kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	logs := &logBuffer{}
	scope := newScope(c.ListenAddrs())
	scope.SetLogger(log.New(logs, "", 0))
	ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
	defer cancel()
	runVQL(ctx, t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=3) },
  brokers=Brokers, topic="vr.events", hold=TRUE, stall_warning=1,
  sasl_mechanism="SCRAM-SHA-256", username="vr", password="WRONG")`)

	var warning string
	for _, l := range strings.Split(logs.String(), "\n") {
		if strings.Contains(l, "waiting") {
			warning = l
		}
	}
	if !strings.Contains(strings.ToLower(warning), "sasl") {
		t.Fatalf("stall warning does not name the cause:\n%s", logs)
	}
	t.Logf("%s", strings.TrimSpace(warning))
}

// Make the broker reject the next n produce requests with
// INVALID_TIMESTAMP, as Kafka 4.0+ does for records stamped more than an
// hour ahead of its clock, then accept as normal.
func rejectTimestamps(c *kfake.Cluster, n int32) {
	var rejected atomic.Int32
	c.ControlKey(int16(kmsg.Produce), func(req kmsg.Request) (kmsg.Response, error, bool) {
		c.KeepControl()
		if rejected.Add(1) > n {
			return nil, nil, false
		}
		preq := req.(*kmsg.ProduceRequest)
		resp := preq.ResponseKind().(*kmsg.ProduceResponse)
		for _, t := range preq.Topics {
			rt := kmsg.NewProduceResponseTopic()
			rt.Topic, rt.TopicID = t.Topic, t.TopicID
			for _, p := range t.Partitions {
				rp := kmsg.NewProduceResponseTopicPartition()
				rp.Partition = p.Partition
				rp.ErrorCode = kerr.InvalidTimestamp.Code
				rt.Partitions = append(rt.Partitions, rp)
			}
			resp.Topics = append(resp.Topics, rt)
		}
		return resp, nil, true
	})
}

// With hold, records rejected for their timestamp are resent with a
// fresh one: all arrive, exactly once, and the log says why.
func TestHoldRetriesRejectedTimestamps(t *testing.T) {
	c := newCluster(t, "vr.events")
	rejectTimestamps(c, 2)

	logs := &logBuffer{}
	scope := newScope(c.ListenAddrs())
	scope.SetLogger(log.New(logs, "", 0))
	rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=20, hosts=2) },
  brokers=Brokers, topic="vr.events", key_field="ClientId", hold=TRUE)`)

	if sent, failed := totals(rows); sent != 20 || failed != 0 {
		t.Fatalf("sent=%d failed=%d, want 20/0\nlogs:\n%s", sent, failed, logs)
	}
	if !strings.Contains(logs.String(), "rejected records for their timestamp") {
		t.Fatalf("no log explaining the timestamp rejections:\n%s", logs)
	}

	seen := map[int64]int{}
	for _, r := range consume(t, c.ListenAddrs(), "vr.events", 20, 5*time.Second) {
		var ev struct{ N int64 }
		json.Unmarshal(r.Value, &ev)
		seen[ev.N]++
	}
	for n := int64(0); n < 20; n++ {
		if seen[n] != 1 {
			t.Fatalf("row %d delivered %d times, want exactly once (all: %v)", n, seen[n], seen)
		}
	}
}

// Without hold, the rejection stays final, as before.
func TestNoHoldFailsRejectedTimestamps(t *testing.T) {
	c := newCluster(t, "vr.events")
	rejectTimestamps(c, 100)
	rows := runVQL(context.Background(), t, newScope(c.ListenAddrs()), `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=5) },
  brokers=Brokers, topic="vr.events", timeout=5)`)
	sent, failed := totals(rows)
	if sent != 0 || failed != 5 {
		t.Fatalf("sent=%d failed=%d, want 0/5", sent, failed)
	}
	if e, _ := rows[0].Get("Error"); !strings.Contains(e.(string), "INVALID_TIMESTAMP") {
		t.Fatalf("error = %v", e)
	}
}

// A NULL column named in header_fields or key_field is left out, not
// sent as the string "Null" (e.g. HuntId on a collection outside a hunt).
func TestNullHeaderAndKeyOmitted(t *testing.T) {
	c := newCluster(t, "vr.events")
	rows := runVQL(context.Background(), t, newScope(c.ListenAddrs()), `
SELECT * FROM kafka_upload(
  query={ SELECT "C.1" AS ClientId, NULL AS HuntId, NULL AS K FROM scope() },
  brokers=Brokers, topic="vr.events", key_field="K",
  header_fields=["ClientId", "HuntId"])`)
	if sent, failed := totals(rows); sent != 1 || failed != 0 {
		t.Fatalf("sent=%d failed=%d, want 1/0", sent, failed)
	}
	recs := consume(t, c.ListenAddrs(), "vr.events", 1, 5*time.Second)
	if len(recs) != 1 {
		t.Fatalf("consumed %d records, want 1", len(recs))
	}
	if len(recs[0].Headers) != 1 || recs[0].Headers[0].Key != "ClientId" {
		t.Fatalf("headers = %+v, want only ClientId", recs[0].Headers)
	}
	if recs[0].Key != nil {
		t.Fatalf("key = %q, want none", recs[0].Key)
	}
}

// recreateTopic deletes a topic and creates it again under the same
// name, which gives it a new topic ID, as kafka.sh down/up did to the
// local broker.
func recreateTopic(t *testing.T, brokers []string, topic string) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	del := kmsg.NewPtrDeleteTopicsRequest()
	dt := kmsg.NewDeleteTopicsRequestTopic()
	dt.Topic = kmsg.StringPtr(topic)
	del.Topics = append(del.Topics, dt)
	del.TopicNames = append(del.TopicNames, topic)
	if resp, err := del.RequestWith(ctx, cl); err != nil {
		t.Fatal(err)
	} else if e := kerr.ErrorForCode(resp.Topics[0].ErrorCode); e != nil {
		t.Fatalf("delete %s: %v", topic, e)
	}

	create := kmsg.NewPtrCreateTopicsRequest()
	ct := kmsg.NewCreateTopicsRequestTopic()
	ct.Topic = topic
	ct.NumPartitions = 1
	ct.ReplicationFactor = 1
	create.Topics = append(create.Topics, ct)
	if resp, err := create.RequestWith(ctx, cl); err != nil {
		t.Fatal(err)
	} else if e := kerr.ErrorForCode(resp.Topics[0].ErrorCode); e != nil {
		t.Fatalf("create %s: %v", topic, e)
	}
}

// A topic deleted and recreated under the same name while the uploader
// runs (seen on a real broker after kafka.sh down/up). franz-go refuses
// the recreated topic until it is purged. With hold the records are
// resent to the new topic, not dropped; without hold at most the batch
// in flight fails. Either way, later batches reach the new topic.
func TestRecreatedTopic(t *testing.T) {
	for _, hold := range []bool{true, false} {
		t.Run(fmt.Sprintf("hold=%v", hold), func(t *testing.T) {
			c := newCluster(t, "vr.events")
			brokers := c.ListenAddrs()
			logs := &logBuffer{}
			scope := newScope(brokers)
			scope.SetLogger(log.New(logs, "", 0))

			arg := &KafkaUploadArgs{Brokers: brokers, Topic: "vr.events",
				Hold: hold, Timeout: 5, StallWarning: -1, Compression: "none"}
			cl, err := NewClient(arg, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cl.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := make(chan vfilter.Row, 1)
			u := &Uploader{Client: cl, Scope: scope, Arg: arg, Output: out,
				LastError: &LastError{}, ctx: ctx}
			send := func(value string) (int, int) {
				u.buffer = []*kgo.Record{{Value: []byte(value)}}
				u.flush(ctx)
				return totals([]*ordereddict.Dict{(<-out).(*ordereddict.Dict)})
			}

			if sent, failed := send("before"); sent != 1 || failed != 0 {
				t.Fatalf("before: sent=%d failed=%d", sent, failed)
			}
			recreateTopic(t, brokers, "vr.events")

			sent, failed := send("during")
			if hold && (sent != 1 || failed != 0) {
				t.Fatalf("hold: first batch after recreation sent=%d failed=%d, want 1/0; log:\n%s",
					sent, failed, logs)
			}
			if sent, failed := send("after"); sent != 1 || failed != 0 {
				t.Fatalf("second batch after recreation sent=%d failed=%d, want 1/0; log:\n%s",
					sent, failed, logs)
			}

			want := 1
			if hold {
				want = 2
			}
			recs := consume(t, brokers, "vr.events", want, 5*time.Second)
			if len(recs) < want {
				t.Fatalf("new topic has %d records, want %d", len(recs), want)
			}
			t.Logf("hold=%v: first batch after recreation sent=%d failed=%d; new topic has %d records",
				hold, sent, failed, len(recs))
		})
	}
}

// LastError reports only errors logged after a batch took its mark,
// without relying on the clock: on Windows an error logged right after
// the mark can carry the very same timestamp.
func TestLastErrorSinceMark(t *testing.T) {
	le := &LastError{}
	le.Log(kgo.LogLevelWarn, "old problem", "err", "stale")
	mark := le.Mark()
	if got := le.Since(mark); got != "" {
		t.Fatalf("error logged before the mark was reported: %q", got)
	}
	le.Log(kgo.LogLevelWarn, "unable to initialize sasl", "err", "SASL_AUTHENTICATION_FAILED")
	if got := le.Since(mark); got != "unable to initialize sasl: SASL_AUTHENTICATION_FAILED" {
		t.Fatalf("Since(mark) = %q", got)
	}
	le.Log(kgo.LogLevelInfo, "not an error")
	if got := le.Since(le.Mark()); got != "" {
		t.Fatalf("info-level log counted as an error: %q", got)
	}
	var none *LastError
	if none.Since(none.Mark()) != "" {
		t.Fatal("nil LastError should report nothing")
	}
}
