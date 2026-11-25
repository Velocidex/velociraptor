package kafka

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/twmb/franz-go/pkg/kgo"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// kafka_consume() reads records from Kafka topics and emits them as
// rows: the reverse of kafka_upload(). In a server event artifact the
// rows are stored like any monitoring data; a query can also act on
// each record (collect_client(), alerts) and store little or nothing.
type KafkaConsumePlugin struct{}

func (self KafkaConsumePlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "kafka_consume",
		Doc:      "Read records from Kafka topics as rows.",
		Version:  1,
		ArgType:  type_map.AddType(scope, &KafkaConsumeArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.NETWORK).Build(),
	}
}

func (self KafkaConsumePlugin) Call(ctx context.Context,
	scope vfilter.Scope, args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "kafka_consume", args)()

		err := vql_subsystem.CheckAccess(scope, acls.NETWORK)
		if err != nil {
			scope.Log("kafka_consume: %v", err)
			return
		}

		arg := &KafkaConsumeArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("kafka_consume: %v", err)
			return
		}

		// The same Kafka Creds secret, TLS setup and client
		// certificate handling as kafka_upload().
		conn := arg.Conn()
		err = mergeSecret(ctx, scope, conn)
		if err != nil {
			scope.Log("kafka_consume: %v", err)
			return
		}
		var tls_config *tls.Config
		tls_config, err = tlsConfig(scope, conn)
		if err != nil {
			scope.Log("kafka_consume: %v", err)
			return
		}
		err = ApplyClientCert(tls_config, conn)
		if err != nil {
			scope.Log("kafka_consume: %v", err)
			return
		}

		c := &Consumer{
			Scope:     scope,
			Arg:       arg,
			Output:    output_chan,
			LastError: &LastError{},
		}
		opts := append(c.GroupHooks(), kgo.WithLogger(c.LastError))
		client, err := NewConsumerClient(arg, conn, tls_config, opts...)
		if err != nil {
			scope.Log("kafka_consume: %v", err)
			return
		}
		defer client.Close()
		c.Client = client
		c.Run(ctx)
	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&KafkaConsumePlugin{})
}

type KafkaConsumeArgs struct {
	Topics       []string `vfilter:"required,field=topics,doc=Topics to read. Names starting with ^ are regular expressions (then all must be)."`
	Group        string   `vfilter:"optional,field=group,doc=Consumer group. Records are committed once they have become rows, so a stopped query resumes where it left off. Without a group nothing is committed and every run begins at start."`
	Start        string   `vfilter:"optional,field=start,doc=Where to begin when the group has no committed position, or with no group: latest (default) or earliest."`
	Decode       string   `vfilter:"optional,field=decode,doc=json (default): a JSON object's fields become columns; raw: the value as text in a Value column."`
	MaxRecords   int64    `vfilter:"optional,field=max_records,doc=Stop after this many records (default: no limit)."`
	IdleTimeout  int64    `vfilter:"optional,field=idle_timeout,doc=Stop after this many seconds without a new record (default: never). With a group it counts from when the group assigns partitions, which can take a few seconds."`
	StallWarning int64    `vfilter:"optional,field=stall_warning,doc=While no records arrive, log the client's last error at most every this many seconds (default 60, negative disables)."`

	Brokers  []string `vfilter:"optional,field=brokers,doc=Seed brokers (host:port). Required unless a secret supplies them."`
	Secret   string   `vfilter:"optional,field=secret,doc=Name of a 'Kafka Creds' secret supplying brokers and authentication."`
	ClientID string   `vfilter:"optional,field=client_id,doc=Kafka client id (default velociraptor)."`

	UseTLS     bool   `vfilter:"optional,field=use_tls,doc=Connect to the brokers over TLS."`
	RootCA     string `vfilter:"optional,field=root_ca,doc=PEM CA certificate(s) to trust in addition to the system pool."`
	SkipVerify bool   `vfilter:"optional,field=skip_verify,doc=Disable TLS certificate verification."`
	ClientCert string `vfilter:"optional,field=client_cert,doc=PEM client certificate to present to the brokers (mutual TLS). Needs client_key and use_tls."`
	ClientKey  string `vfilter:"optional,field=client_key,doc=PEM private key for client_cert (unencrypted PKCS#1, PKCS#8 or EC)."`

	SASLMechanism string `vfilter:"optional,field=sasl_mechanism,doc=PLAIN, SCRAM-SHA-256 or SCRAM-SHA-512."`
	Username      string `vfilter:"optional,field=username,doc=SASL username."`
	Password      string `vfilter:"optional,field=password,doc=SASL password."`
}

// Conn returns the connection settings in the form the shared helpers
// (secrets, TLS, client certificates, SASL) take, so both plugins honour
// the same Kafka Creds fields in the same way.
func (self *KafkaConsumeArgs) Conn() *KafkaUploadArgs {
	return &KafkaUploadArgs{
		Brokers:       self.Brokers,
		Secret:        self.Secret,
		ClientID:      self.ClientID,
		UseTLS:        self.UseTLS,
		RootCA:        self.RootCA,
		SkipVerify:    self.SkipVerify,
		ClientCert:    self.ClientCert,
		ClientKey:     self.ClientKey,
		SASLMechanism: self.SASLMechanism,
		Username:      self.Username,
		Password:      self.Password,
	}
}

func NewConsumerClient(arg *KafkaConsumeArgs, conn *KafkaUploadArgs,
	tls_config *tls.Config, extra ...kgo.Opt) (*kgo.Client, error) {
	if len(conn.Brokers) == 0 {
		return nil, fmt.Errorf("no brokers: set brokers= or use a secret that has them")
	}
	if len(arg.Topics) == 0 {
		return nil, fmt.Errorf("topics= is required")
	}
	if conn.ClientID == "" {
		conn.ClientID = "velociraptor"
	}
	if arg.StallWarning == 0 {
		arg.StallWarning = 60
	}
	switch strings.ToLower(arg.Decode) {
	case "", "json", "raw":
	default:
		return nil, fmt.Errorf("decode must be json or raw, not %q", arg.Decode)
	}

	// A fetch the broker has no data for is held open ("long poll")
	// for up to this long. Partitions that become ready meanwhile wait
	// for the next fetch, which happens when a group assigns several
	// partitions and their start positions resolve at slightly
	// different times. franz-go's default of 5s would let a short
	// idle_timeout expire with records waiting, so keep it short.
	max_wait := time.Second
	if idle := time.Duration(arg.IdleTimeout) * time.Second / 2; idle > 0 && idle < max_wait {
		max_wait = idle
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(conn.Brokers...),
		kgo.ClientID(conn.ClientID),
		kgo.ConsumeTopics(arg.Topics...),
		kgo.FetchMaxWait(max_wait),
	}

	patterns := 0
	for _, t := range arg.Topics {
		if strings.HasPrefix(t, "^") {
			patterns++
		}
	}
	if patterns > 0 {
		if patterns != len(arg.Topics) {
			return nil, fmt.Errorf("topics: either all names are regular expressions (starting with ^) or none")
		}
		opts = append(opts, kgo.ConsumeRegex())
	}

	switch strings.ToLower(arg.Start) {
	case "", "latest":
		opts = append(opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))
	case "earliest":
		opts = append(opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	default:
		return nil, fmt.Errorf("start must be latest or earliest, not %q", arg.Start)
	}

	if arg.Group != "" {
		opts = append(opts,
			kgo.ConsumerGroup(arg.Group),
			// We commit ourselves, only records that have become rows.
			kgo.DisableAutoCommit(),
			// Hold off rebalances between a poll and its commit, so
			// partitions are never handed to another member with
			// rows emitted here but not yet committed.
			kgo.BlockRebalanceOnPoll())
	}

	if tls_config != nil {
		opts = append(opts, kgo.DialTLSConfig(tls_config))
	}
	mechanism, err := parseSASL(conn.SASLMechanism, conn.Username, conn.Password)
	if err != nil {
		return nil, err
	}
	if mechanism != nil {
		opts = append(opts, kgo.SASL(mechanism))
	}

	opts = append(opts, extra...)
	return kgo.NewClient(opts...)
}

type Consumer struct {
	Client *kgo.Client
	Scope  vfilter.Scope
	Arg    *KafkaConsumeArgs
	Output chan vfilter.Row

	// Optional: lets an idle consumer say why nothing is arriving.
	LastError *LastError

	// When the group last assigned this consumer partitions (unix
	// nanoseconds; 0 until the first assignment). Partitions can arrive
	// in stages, so each assignment restarts the idle_timeout clock.
	assigned_at atomic.Int64
}

// GroupHooks returns the client options that tell the consumer when its
// group has assigned it partitions. Pass them to NewConsumerClient.
func (self *Consumer) GroupHooks() []kgo.Opt {
	if self.Arg.Group == "" {
		return nil
	}
	return []kgo.Opt{kgo.OnPartitionsAssigned(
		func(_ context.Context, _ *kgo.Client, assigned map[string][]int32) {
			self.assigned_at.Store(time.Now().UnixNano())
			self.Scope.Log("kafka_consume: group %v assigned %v",
				self.Arg.Group, describePartitions(assigned))
		})}
}

// How long one poll waits before the loop checks idle_timeout and
// stall warnings again.
var ConsumePollInterval = 2 * time.Second

// Joining a consumer group takes a while: Kafka holds a new group for
// group.initial.rebalance.delay.ms (3s by default), and a member that
// left without saying so holds up the next assignment until its session
// times out (45s by default). idle_timeout only starts counting once the
// group has assigned partitions, but no later than this, so a broker
// that never answers still ends the query.
var GroupJoinGrace = 60 * time.Second

func (self *Consumer) Run(ctx context.Context) {
	in_group := self.Arg.Group != ""
	joining := in_group
	idle := time.Duration(self.Arg.IdleTimeout) * time.Second
	stall := time.Duration(self.Arg.StallWarning) * time.Second

	var count int64
	started := time.Now()
	last_record := started
	last_warning := started
	error_mark := self.LastError.Mark()
	var seen_assignment int64

	for {
		if at := self.assigned_at.Load(); at > seen_assignment {
			seen_assignment = at
			joining = false
			last_record = time.Now()
		}

		wait := ConsumePollInterval
		if idle > 0 {
			remaining := idle - time.Since(last_record)
			if joining {
				remaining = GroupJoinGrace - time.Since(started)
				if remaining < idle-time.Since(last_record) {
					remaining = idle - time.Since(last_record)
				}
			}
			if remaining <= 0 {
				// Stopping for lack of records: if a problem is why
				// nothing arrived, say so, even when idle_timeout ends
				// the query before the first stall warning was due.
				if e := self.LastError.Since(error_mark); e != "" {
					self.Scope.Log("kafka_consume: stopping after %v without records (last error: %v)",
						time.Since(last_record).Round(time.Second), e)
				}
				return
			}
			if remaining < wait {
				wait = remaining
			}
		}
		poll_ctx, cancel := context.WithTimeout(ctx, wait)
		fetches := self.Client.PollFetches(poll_ctx)
		cancel()

		if fetches.IsClientClosed() {
			return
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return
			}
			self.Scope.Log("kafka_consume: %v partition %v: %v", topic, partition, err)
		})

		var emitted []*kgo.Record
		stop := ctx.Err() != nil
		for _, record := range fetches.Records() {
			if stop {
				break
			}
			select {
			case <-ctx.Done():
				stop = true
			case self.Output <- self.toRow(record):
				emitted = append(emitted, record)
				count++
				if self.Arg.MaxRecords > 0 && count >= self.Arg.MaxRecords {
					stop = true
				}
			}
		}

		if len(emitted) > 0 {
			last_record = time.Now()
			error_mark = self.LastError.Mark()
			if in_group {
				self.commit(emitted)
			}
		} else if stall > 0 && time.Since(last_record) >= stall &&
			time.Since(last_warning) >= stall {
			if e := self.LastError.Since(error_mark); e != "" {
				self.Scope.Log("kafka_consume: no records for %v (last error: %v)",
					time.Since(last_record).Round(time.Second), e)
				error_mark = self.LastError.Mark()
			}
			last_warning = time.Now()
		}
		if in_group {
			self.Client.AllowRebalance()
		}
		if stop || ctx.Err() != nil {
			return
		}
	}
}

// commit records the position after the given records, which have all
// been emitted as rows. A detached context, so that rows already
// emitted are committed even while the query is being cancelled.
func (self *Consumer) commit(records []*kgo.Record) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := self.Client.CommitRecords(ctx, records...)
	if err != nil {
		self.Scope.Log("kafka_consume: committing %d records: %v "+
			"(they may be read again)", len(records), err)
	}
}

// A record becomes a row: with decode=json, a JSON object's
// fields become columns (decoded as Velociraptor decodes JSON elsewhere:
// whole numbers as uint64 or int64, nested objects as dicts); anything
// else lands in Value. Record metadata
// goes in underscore columns, capitalised so that a row passed on to
// kafka_upload() is not routed by a _topic column by accident.
func (self *Consumer) toRow(record *kgo.Record) *ordereddict.Dict {
	row := ordereddict.NewDict()
	decoded := false
	if !strings.EqualFold(self.Arg.Decode, "raw") {
		obj := ordereddict.NewDict()
		if err := obj.UnmarshalJSON(record.Value); err == nil {
			for _, k := range obj.Keys() {
				v, _ := obj.Get(k)
				row.Set(k, v)
			}
			decoded = true
		}
	}
	if !decoded {
		row.Set("Value", string(record.Value))
	}

	var key interface{}
	if record.Key != nil {
		key = string(record.Key)
	}
	headers := ordereddict.NewDict()
	for _, h := range record.Headers {
		headers.Set(h.Key, string(h.Value))
	}
	return row.
		Set("_Topic", record.Topic).
		Set("_Partition", record.Partition).
		Set("_Offset", record.Offset).
		Set("_Key", key).
		Set("_Time", record.Timestamp.UTC()).
		Set("_Headers", headers)
}

// describePartitions renders an assignment as "topic[0 1 2]", sorted so
// the log reads the same each time.
func describePartitions(assigned map[string][]int32) string {
	topics := make([]string, 0, len(assigned))
	for topic := range assigned {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	parts := make([]string, 0, len(topics))
	for _, topic := range topics {
		partitions := append([]int32(nil), assigned[topic]...)
		sort.Slice(partitions, func(i, j int) bool { return partitions[i] < partitions[j] })
		parts = append(parts, fmt.Sprintf("%v%v", topic, partitions))
	}
	return strings.Join(parts, ", ")
}
