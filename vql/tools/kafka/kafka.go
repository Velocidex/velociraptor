// Package kafka provides the kafka_upload() VQL plugin: it uploads the
// rows of a query to a Kafka topic, batching by chunk_size/wait_time
// like elastic_upload().
package kafka

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type KafkaUploadPlugin struct{}

func (self KafkaUploadPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "kafka_upload",
		Doc:      "Upload rows from a query to a Kafka topic.",
		Version:  1,
		ArgType:  type_map.AddType(scope, &KafkaUploadArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.NETWORK).Build(),
	}
}

func (self KafkaUploadPlugin) Call(ctx context.Context,
	scope vfilter.Scope, args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "kafka_upload", args)()

		err := vql_subsystem.CheckAccess(scope, acls.NETWORK)
		if err != nil {
			scope.Log("kafka_upload: %v", err)
			return
		}

		arg := &KafkaUploadArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("kafka_upload: %v", err)
			return
		}

		err = mergeSecret(ctx, scope, arg)
		if err != nil {
			scope.Log("kafka_upload: %v", err)
			return
		}

		tls_config, err := tlsConfig(scope, arg)
		if err != nil {
			scope.Log("kafka_upload: %v", err)
			return
		}
		err = ApplyClientCert(tls_config, arg)
		if err != nil {
			scope.Log("kafka_upload: %v", err)
			return
		}

		last_error := &LastError{}
		client, err := NewClient(arg, tls_config, kgo.WithLogger(last_error))
		if err != nil {
			scope.Log("kafka_upload: %v", err)
			return
		}
		defer client.Close()

		u := &Uploader{
			Client:    client,
			Scope:     scope,
			Arg:       arg,
			Output:    output_chan,
			Marshal:   json.Marshal,
			LastError: last_error,
		}
		u.Run(ctx)
	}()

	return output_chan
}

// Same policy as elastic_upload(): when the server requires VQL to use
// secrets, a query without secret= is refused; with one, the Kafka
// Creds secret supplies the brokers and credentials.
func mergeSecret(ctx context.Context, scope vfilter.Scope, arg *KafkaUploadArgs) error {
	config_obj, on_server := vql_subsystem.GetServerConfig(scope)

	if arg.Secret == "" {
		if on_server && config_obj.Security != nil &&
			config_obj.Security.VqlMustUseSecrets {
			return utils.SecretsEnforced
		}
		return nil
	}

	if !on_server {
		return fmt.Errorf("secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return err
	}

	// GetSecret checks the caller is allowed to use this secret.
	s, err := secrets_service.GetSecret(ctx,
		vql_subsystem.GetPrincipal(scope), KafkaSecretType, arg.Secret)
	if err != nil {
		return err
	}

	return ApplySecret(arg, s.Data)
}

// tlsConfig is nil unless use_tls is set. It uses Velociraptor's TLS
// setup, so the deployment CA and certificate verification policy are
// honoured. Shared by kafka_upload() and kafka_consume().
func tlsConfig(scope vfilter.Scope, arg *KafkaUploadArgs) (*tls.Config, error) {
	if !arg.UseTLS {
		return nil, nil
	}
	config_obj, ok := artifacts.GetConfig(scope)
	if !ok {
		// GetTlsConfig needs a config; like the HTTP transport, fall
		// back to an empty one.
		config_obj = &config_proto.ClientConfig{}
	}
	tls_config, err := networking.GetTlsConfig(config_obj, arg.RootCA)
	if err == nil && arg.SkipVerify {
		err = networking.EnableSkipVerify(tls_config, config_obj)
	}
	return tls_config, err
}

func init() {
	vql_subsystem.RegisterPlugin(&KafkaUploadPlugin{})
}

type KafkaUploadArgs struct {
	Query        vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for rows to upload."`
	Brokers      []string            `vfilter:"optional,field=brokers,doc=Seed brokers (host:port). Required unless a secret supplies them."`
	Secret       string              `vfilter:"optional,field=secret,doc=Name of a 'Kafka Creds' secret supplying brokers and authentication."`
	Topic        string              `vfilter:"optional,field=topic,doc=Topic to produce to. If unset, each row must carry a _topic column."`
	KeyField     string              `vfilter:"optional,field=key_field,doc=Row column used as the record key; decides the partition (e.g. ClientId)."`
	ChunkSize    int64               `vfilter:"optional,field=chunk_size,doc=Rows per batch (default 1000)."`
	WaitTime     int64               `vfilter:"optional,field=wait_time,doc=Maximum seconds to buffer before flushing (default 2)."`
	Timeout      int64               `vfilter:"optional,field=timeout,doc=Seconds to wait for a batch to be acknowledged before failing it (default 30). Ignored when hold is set."`
	Hold         bool                `vfilter:"optional,field=hold,doc=Never drop records: wait for the broker indefinitely instead of failing batches after timeout. Recommended for monitoring artifacts."`
	StallWarning int64               `vfilter:"optional,field=stall_warning,doc=Log a warning every this many seconds while a batch waits for the broker (default 60, negative disables)."`
	Compression  string              `vfilter:"optional,field=compression,doc=none, gzip, snappy, lz4 or zstd (default zstd)."`
	Headers      *ordereddict.Dict   `vfilter:"optional,field=headers,doc=Static headers added to every record."`
	HeaderFields []string            `vfilter:"optional,field=header_fields,doc=Row columns copied into per-record headers (they stay in the payload too), e.g. Artifact, ClientId."`
	ClientID     string              `vfilter:"optional,field=client_id,doc=Kafka client id (default velociraptor)."`

	UseTLS     bool   `vfilter:"optional,field=use_tls,doc=Connect to the brokers over TLS."`
	RootCA     string `vfilter:"optional,field=root_ca,doc=PEM CA certificate(s) to trust in addition to the system pool."`
	SkipVerify bool   `vfilter:"optional,field=skip_verify,doc=Disable TLS certificate verification."`
	ClientCert string `vfilter:"optional,field=client_cert,doc=PEM client certificate to present to the brokers (mutual TLS). Needs client_key and use_tls."`
	ClientKey  string `vfilter:"optional,field=client_key,doc=PEM private key for client_cert (unencrypted PKCS#1, PKCS#8 or EC)."`

	SASLMechanism string `vfilter:"optional,field=sasl_mechanism,doc=PLAIN, SCRAM-SHA-256 or SCRAM-SHA-512."`
	Username      string `vfilter:"optional,field=username,doc=SASL username."`
	Password      string `vfilter:"optional,field=password,doc=SASL password."`
}

// LastError keeps the most recent problem the Kafka client logged, so a
// stall warning can say why records are waiting (bad credentials, TLS
// mismatch, connection refused) rather than only that they are.
type LastError struct {
	mu  sync.Mutex
	msg string
	// Counts errors logged so far. Batches compare counts rather than
	// timestamps: on Windows the clock only advances once per timer
	// tick (up to ~16ms), so an error logged right after a batch starts
	// can carry the very same timestamp.
	seq uint64
}

func (self *LastError) Level() kgo.LogLevel { return kgo.LogLevelWarn }

func (self *LastError) Log(level kgo.LogLevel, msg string, keyvals ...interface{}) {
	if level == kgo.LogLevelNone || level > kgo.LogLevelWarn {
		return
	}
	for i := 0; i+1 < len(keyvals); i += 2 {
		if key, _ := keyvals[i].(string); key == "err" {
			msg = fmt.Sprintf("%s: %v", msg, keyvals[i+1])
		}
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	self.msg = msg
	self.seq++
}

// Mark returns a position to pass to Since later.
func (self *LastError) Mark() uint64 {
	if self == nil {
		return 0
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.seq
}

// The last error logged after mark was taken, or "".
func (self *LastError) Since(mark uint64) string {
	if self == nil {
		return ""
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	if self.seq > mark {
		return self.msg
	}
	return ""
}

func NewClient(arg *KafkaUploadArgs, tls_config *tls.Config,
	extra ...kgo.Opt) (*kgo.Client, error) {
	if arg.ChunkSize <= 0 {
		arg.ChunkSize = 1000
	}
	if arg.WaitTime <= 0 {
		arg.WaitTime = 2
	}
	if arg.Timeout <= 0 {
		arg.Timeout = 30
	}
	if arg.StallWarning == 0 {
		arg.StallWarning = 60
	}
	if arg.ClientID == "" {
		arg.ClientID = "velociraptor"
	}
	if len(arg.Brokers) == 0 {
		return nil, fmt.Errorf("no brokers: set brokers= or use a secret that has them")
	}

	codec, err := parseCompression(arg.Compression)
	if err != nil {
		return nil, err
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(arg.Brokers...),
		kgo.ClientID(arg.ClientID),
		kgo.ProducerBatchCompression(codec),
		// We already batch by chunk_size/wait_time, so the client's
		// own linger (10ms by default) would only add latency to
		// every batch.
		kgo.ProducerLinger(0),
	}
	// Without a delivery timeout the client retries until the broker
	// answers, which is what hold mode wants.
	if !arg.Hold {
		opts = append(opts, kgo.RecordDeliveryTimeout(
			time.Duration(arg.Timeout)*time.Second))
	}
	if arg.Topic != "" {
		opts = append(opts, kgo.DefaultProduceTopic(arg.Topic))
	}
	if tls_config != nil {
		opts = append(opts, kgo.DialTLSConfig(tls_config))
	}

	mechanism, err := parseSASL(arg.SASLMechanism, arg.Username, arg.Password)
	if err != nil {
		return nil, err
	}
	if mechanism != nil {
		opts = append(opts, kgo.SASL(mechanism))
	}

	opts = append(opts, extra...)
	return kgo.NewClient(opts...)
}

type Uploader struct {
	Client  *kgo.Client
	Scope   vfilter.Scope
	Arg     *KafkaUploadArgs
	Output  chan vfilter.Row
	Marshal func(v interface{}) ([]byte, error)

	// Optional: makes stall warnings say why the broker is not answering.
	LastError *LastError

	ctx     context.Context
	headers []kgo.RecordHeader
	buffer  []*kgo.Record
}

func (self *Uploader) Run(ctx context.Context) {
	self.ctx = ctx
	self.headers = buildHeaders(self.Arg.Headers)

	row_chan := self.Arg.Query.Eval(ctx, self.Scope)
	wait := time.Duration(self.Arg.WaitTime) * time.Second
	next_flush := time.After(wait)

	for {
		select {
		case <-ctx.Done():
			// The query was cancelled (e.g. the artifact was stopped).
			// Still try to deliver what we hold; flush bounds the wait.
			self.flush(ctx)
			return

		case row, ok := <-row_chan:
			if !ok {
				self.flush(ctx)
				return
			}
			record, err := self.toRecord(row)
			if err != nil {
				self.Scope.Log("kafka_upload: %v", err)
				continue
			}
			self.buffer = append(self.buffer, record)
			if int64(len(self.buffer)) >= self.Arg.ChunkSize {
				self.flush(ctx)
				next_flush = time.After(wait)
			}

		case <-next_flush:
			self.flush(ctx)
			next_flush = time.After(wait)
		}
	}
}

// topicRecreated reports a record that failed because its topic was
// deleted and recreated under the same name. Kafka gives the new topic a
// new ID, and franz-go deliberately refuses to write to it (failing the
// record with UNKNOWN_TOPIC_ID) until the topic is purged from the
// client, which then looks it up afresh. Records still buffered for the
// topic when it is purged fail with franz-go's "topic purged" error.
func topicRecreated(err error) bool {
	return errors.Is(err, kerr.UnknownTopicID) ||
		strings.Contains(err.Error(), "topic purged while buffered")
}

func (self *Uploader) flush(ctx context.Context) {
	if len(self.buffer) == 0 {
		return
	}
	batch := self.buffer
	self.buffer = nil

	// The records' own context. Cancelling the query must not cancel
	// records we already hold, so it is detached from ctx and bounded
	// only by the timeout (or not at all in hold mode).
	produce_ctx, cancel := context.WithCancel(context.Background())
	if !self.Arg.Hold {
		// The client's own delivery timeout (the same value) reports why
		// records failed, so give it a head start; this is a backstop.
		produce_ctx, cancel = context.WithTimeout(context.Background(),
			time.Duration(self.Arg.Timeout)*time.Second+5*time.Second)
	}
	defer cancel()

	start := time.Now()
	// Errors the client logs from here on concern this batch.
	error_mark := self.LastError.Mark()

	// Buffered so a promise that fires after we stop waiting (during
	// shutdown) never blocks the client. Each record has at most one
	// produce in flight, so len(batch) is always enough room.
	type produced struct {
		record *kgo.Record
		err    error
	}
	results := make(chan produced, len(batch))
	produce := func(record *kgo.Record) {
		self.Client.Produce(produce_ctx, record,
			func(r *kgo.Record, err error) { results <- produced{r, err} })
	}
	for _, record := range batch {
		produce(record)
	}

	// In hold mode, some failures are resent after a backoff instead
	// of being dropped:
	//   * INVALID_TIMESTAMP: Kafka 4.0+ rejects timestamps more than an
	//     hour ahead of the broker's clock, so this is clock skew between
	//     the Velociraptor server and the broker; resent with a fresh
	//     timestamp, it clears once the clocks agree.
	//   * a topic deleted and recreated under the same name: see
	//     topicRecreated.
	var retry_records []*kgo.Record
	var retry_at <-chan time.Time
	retry_backoff := time.Second
	reject_reason := ""

	var stall <-chan time.Time
	if self.Arg.StallWarning > 0 {
		ticker := time.NewTicker(time.Duration(self.Arg.StallWarning) * time.Second)
		defer ticker.Stop()
		stall = ticker.C
	}

	// Once the query is cancelled we give the broker a grace period,
	// then stop waiting so stopping an artifact can never hang on a
	// dead broker.
	cancelled := ctx.Done()
	var give_up <-chan time.Time

	sent, failed := 0, 0
	var first_err error
	purged := map[string]bool{}

wait:
	for done := 0; done < len(batch); {
		select {
		case res := <-results:
			if res.err != nil && errors.Is(res.err, kerr.UnknownTopicID) &&
				!purged[res.record.Topic] {
				// Whether or not we hold, later batches must reach
				// the new topic.
				purged[res.record.Topic] = true
				self.Scope.Log("kafka_upload: topic %v was deleted and recreated; "+
					"switching to the new topic", res.record.Topic)
				self.Client.PurgeTopicsFromProducing(res.record.Topic)
			}
			if res.err != nil && self.Arg.Hold &&
				(errors.Is(res.err, kerr.InvalidTimestamp) || topicRecreated(res.err)) {
				if reject_reason == "" && errors.Is(res.err, kerr.InvalidTimestamp) {
					self.Scope.Log("kafka_upload: broker rejected records for their timestamp (%v); "+
						"holding and retrying. Check the clocks on the Velociraptor server and the brokers.",
						res.err)
				}
				reject_reason = res.err.Error()
				retry_records = append(retry_records, res.record)
				if retry_at == nil {
					retry_at = time.After(retry_backoff)
					retry_backoff = min(retry_backoff*2, 30*time.Second)
				}
				continue
			}
			done++
			if res.err != nil {
				failed++
				if first_err == nil {
					first_err = res.err
					// A bare deadline says nothing; add what the client
					// last complained about.
					if errors.Is(res.err, context.DeadlineExceeded) {
						if e := self.LastError.Since(error_mark); e != "" {
							first_err = fmt.Errorf("%w (last error: %s)", res.err, e)
						}
					}
				}
			} else {
				sent++
			}

		case <-retry_at:
			retry_at = nil
			for _, r := range retry_records {
				// A new record, so the client stamps it with the time
				// of this attempt.
				produce(&kgo.Record{
					Topic: r.Topic, Key: r.Key, Value: r.Value, Headers: r.Headers})
			}
			retry_records = nil

		case <-stall:
			reason := ""
			if reject_reason != "" {
				reason = " (last error: " + reject_reason + ")"
			} else if e := self.LastError.Since(error_mark); e != "" {
				reason = " (last error: " + e + ")"
			}
			self.Scope.Log("kafka_upload: %d of %d records waiting %v for the broker%s",
				len(batch)-done, len(batch), time.Since(start).Round(time.Second), reason)

		case <-cancelled:
			cancelled = nil
			give_up = time.After(shutdownGrace(self.Arg))

		case <-give_up:
			pending := len(batch) - done
			failed += pending
			if first_err == nil {
				first_err = fmt.Errorf("stopped with %d records unacknowledged by the broker", pending)
			}
			break wait
		}
	}

	result := ordereddict.NewDict().
		Set("Time", start.UTC()).
		Set("Sent", sent).
		Set("Failed", failed).
		Set("DurationMs", time.Since(start).Milliseconds()).
		Set("Error", "")
	if first_err != nil {
		result.Set("Error", first_err.Error())
		self.Scope.Log("kafka_upload: %d records failed: %v", failed, first_err)
	}

	// If the query was cancelled nobody is reading results any more,
	// so don't block on the send.
	select {
	case self.Output <- result:
	case <-self.ctx.Done():
	}
}

// How long to keep trying to deliver held records after the query is
// cancelled. Without hold the records time out on their own; with hold
// they never would, so use a fixed bound.
func shutdownGrace(arg *KafkaUploadArgs) time.Duration {
	if arg.Hold {
		return HoldShutdownGrace
	}
	return time.Duration(arg.Timeout) * time.Second
}

// A variable so tests can shorten it.
var HoldShutdownGrace = 10 * time.Second

// Turn a VQL row into a Kafka record. The value is the row as JSON,
// minus the _topic routing column if one is present.
func (self *Uploader) toRecord(row vfilter.Row) (*kgo.Record, error) {
	dict := rowToDict(self.Scope, row)
	record := &kgo.Record{Topic: self.Arg.Topic}

	if topic, pres := dict.Get("_topic"); pres {
		record.Topic = fmt.Sprint(topic)
		dict.Delete("_topic")
	}
	if record.Topic == "" {
		return nil, fmt.Errorf("row has no _topic column and no topic was given")
	}

	if self.Arg.KeyField != "" {
		if key, pres := dict.Get(self.Arg.KeyField); pres && !isNull(key) {
			record.Key = []byte(fmt.Sprint(key))
		}
	}

	value, err := self.Marshal(dict)
	if err != nil {
		return nil, err
	}
	record.Value = value

	// Copy the static headers so per-record headers never alias them.
	record.Headers = append([]kgo.RecordHeader(nil), self.headers...)
	for _, field := range self.Arg.HeaderFields {
		if v, pres := dict.Get(field); pres && !isNull(v) {
			record.Headers = append(record.Headers, kgo.RecordHeader{
				Key: field, Value: []byte(fmt.Sprint(v))})
		}
	}
	return record, nil
}

// A missing value, or VQL's NULL (which is not Go's nil): such key and
// header fields are left out rather than sent as the string "Null".
func isNull(v interface{}) bool {
	switch v.(type) {
	case nil, types.Null, *types.Null:
		return true
	}
	return false
}

func rowToDict(scope vfilter.Scope, row vfilter.Row) *ordereddict.Dict {
	out := ordereddict.NewDict()
	if d, ok := row.(*ordereddict.Dict); ok {
		// Copy, so deleting _topic doesn't modify the caller's row.
		for _, k := range d.Keys() {
			v, _ := d.Get(k)
			out.Set(k, v)
		}
		return out
	}
	for _, member := range scope.GetMembers(row) {
		v, _ := scope.Associative(row, member)
		out.Set(member, v)
	}
	return out
}

func buildHeaders(d *ordereddict.Dict) []kgo.RecordHeader {
	if d == nil {
		return nil
	}
	headers := []kgo.RecordHeader{}
	for _, k := range d.Keys() {
		v, _ := d.Get(k)
		headers = append(headers, kgo.RecordHeader{
			Key: k, Value: []byte(fmt.Sprint(v))})
	}
	return headers
}

func parseCompression(name string) (kgo.CompressionCodec, error) {
	switch strings.ToLower(name) {
	case "", "zstd":
		return kgo.ZstdCompression(), nil
	case "none":
		return kgo.NoCompression(), nil
	case "gzip":
		return kgo.GzipCompression(), nil
	case "snappy":
		return kgo.SnappyCompression(), nil
	case "lz4":
		return kgo.Lz4Compression(), nil
	}
	return kgo.NoCompression(), fmt.Errorf("unknown compression %q", name)
}

// The secret type kafka_upload(secret=...) reads. In-tree this is
// registered with the secrets service; the fields are KafkaSecretFields.
const KafkaSecretType = constants.KAFKA_CREDS

var KafkaSecretFields = []string{
	"brokers", "sasl_mechanism", "username", "password",
	"use_tls", "root_ca", "skip_verify", "client_cert", "client_key",
}

var secretBool = regexp.MustCompile(`(?i)^\s*(true|y|yes|1)\s*$`)

// ApplySecret overlays a Kafka Creds secret onto the arguments. Fields
// set in the secret win, so credentials and the broker list come from
// the secret even if the query also passes them; fields left empty in
// the secret keep the query's value. Brokers may be one per line or
// comma separated, and lines starting with # are ignored, matching how
// the other Velociraptor secret types store lists.
func ApplySecret(arg *KafkaUploadArgs, data *ordereddict.Dict) error {
	get := func(field string) string {
		v, _ := data.GetString(field)
		return strings.TrimSpace(v)
	}

	var brokers []string
	for _, line := range strings.Split(get("brokers"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, b := range strings.Split(line, ",") {
			if b = strings.TrimSpace(b); b != "" {
				brokers = append(brokers, b)
			}
		}
	}
	if len(brokers) == 0 {
		return fmt.Errorf("secret %q has no brokers", arg.Secret)
	}
	arg.Brokers = brokers

	if v := get("sasl_mechanism"); v != "" {
		arg.SASLMechanism = v
	}
	if v := get("username"); v != "" {
		arg.Username = v
	}
	if v := get("password"); v != "" {
		arg.Password = v
	}
	if v := get("root_ca"); v != "" {
		arg.RootCA = v
	}
	if v := get("use_tls"); v != "" {
		arg.UseTLS = secretBool.MatchString(v)
	}
	if v := get("skip_verify"); v != "" {
		arg.SkipVerify = secretBool.MatchString(v)
	}
	if v := get("client_cert"); v != "" {
		arg.ClientCert = v
	}
	if v := get("client_key"); v != "" {
		arg.ClientKey = v
	}
	return nil
}

// ApplyClientCert sets up mutual TLS: with client_cert and client_key
// the client presents that certificate to the brokers, replacing any
// certificate the TLS setup added on its own. tls_config is nil when
// use_tls is off.
func ApplyClientCert(tls_config *tls.Config, arg *KafkaUploadArgs) error {
	if arg.ClientCert == "" && arg.ClientKey == "" {
		return nil
	}
	if arg.ClientCert == "" || arg.ClientKey == "" {
		return fmt.Errorf("client_cert and client_key must be given together")
	}
	if tls_config == nil {
		return fmt.Errorf("client_cert needs use_tls=TRUE")
	}
	cert, err := tls.X509KeyPair([]byte(arg.ClientCert), []byte(arg.ClientKey))
	if err != nil {
		return fmt.Errorf("unable to load client_cert and client_key: %w", err)
	}
	tls_config.Certificates = []tls.Certificate{cert}
	return nil
}

func parseSASL(mechanism, user, pass string) (sasl.Mechanism, error) {
	switch strings.ToUpper(mechanism) {
	case "":
		return nil, nil
	case "PLAIN":
		return plain.Auth{User: user, Pass: pass}.AsMechanism(), nil
	case "SCRAM-SHA-256":
		return scram.Auth{User: user, Pass: pass}.AsSha256Mechanism(), nil
	case "SCRAM-SHA-512":
		return scram.Auth{User: user, Pass: pass}.AsSha512Mechanism(), nil
	}
	return nil, fmt.Errorf("unknown sasl_mechanism %q", mechanism)
}
