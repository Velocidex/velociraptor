package rsyslog

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/leodido/go-syslog/rfc5424"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type RsyslogFunctionArgs struct {
	Hostname       string            `vfilter:"required,field=hostname,doc=Destination host to connect to."`
	Port           uint64            `vfilter:"optional,field=port,doc=Destination port to connect to. If not specified we use 514"`
	Protocol       string            `vfilter:"optional,field=protocol,doc=Protocol to use, default UDP but can be TCP or TLS"`
	Message        string            `vfilter:"required,field=message,doc=Message to log."`
	Facility       int64             `vfilter:"optional,field=facility,doc=Facility of this message"`
	Severity       int64             `vfilter:"optional,field=severity,doc=Severity of this message"`
	Timestamp      time.Time         `vfilter:"optional,field=timestamp,doc=Timestamp of this message, if omitted we use the current time."`
	SourceHostname string            `vfilter:"optional,field=hostname,doc=Hostname associated with this message. If omitted we use the current hostname."`
	AppName        string            `vfilter:"optional,field=app_name,doc=Application that generated the log"`
	ProcId         string            `vfilter:"optional,field=proc_id,doc=Process ID that generated this log"`
	SdID           string            `vfilter:"optional,field=sd_id,doc=When sending structured data, this is the Structured Data ID"`
	Args           *ordereddict.Dict `vfilter:"optional,field=args,doc=A dict to be interpolated into the message as structured data, according to RFC5424."`
	RootCerts      string            `vfilter:"optional,field=root_ca,doc=As a better alternative to disable_ssl_security, allows root ca certs to be added here."`
}

type RsyslogFunction struct{}

func (self *RsyslogFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "rsyslog", args)()
	arg := &RsyslogFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("rsyslog: %s", err.Error())
		return false
	}

	err = vql_subsystem.CheckAccess(scope, acls.NETWORK)
	if err != nil {
		scope.Log("rsyslog: %s", err)
		return false
	}

	// Go ahead and format the message now
	if arg.Port == 0 {
		arg.Port = 514
	}

	if arg.Protocol == "" {
		arg.Protocol = "udp"
	}

	if arg.Timestamp.IsZero() {
		arg.Timestamp = utils.GetTime().Now()
	}

	if arg.SourceHostname == "" {
		hostname, _ := os.Hostname()
		arg.SourceHostname = hostname
	}

	connect_timeout := time.Minute

	var config_obj *config_proto.ClientConfig

	config_obj_any, ok := scope.Resolve(constants.SCOPE_CONFIG)
	if ok {
		config_obj, ok = config_obj_any.(*config_proto.ClientConfig)
		if !ok {
			scope.Log("rsyslog: invalid config %T", config_obj_any)
			return false
		}
	}

	raddr := net.JoinHostPort(arg.Hostname, fmt.Sprintf("%v", arg.Port))

	pool := GetPool(ctx, scope)

	// This will be closed later by the connection pool.
	logger, err := pool.Dial(config_obj, strings.ToLower(arg.Protocol),
		raddr, arg.RootCerts, connect_timeout)
	if err != nil {
		scope.Log("rsyslog: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.SdID == "" {
		arg.SdID = "msg@123"
	}

	// Format the message for sending.
	message := &rfc5424.SyslogMessage{}
	message.SetVersion(1).
		SetMessage(arg.Message).
		SetPriority(uint8((arg.Facility << 3) | arg.Severity)).
		SetTimestamp(arg.Timestamp.Format(time.RFC3339)).
		SetHostname(arg.SourceHostname).
		SetAppname(arg.AppName).
		SetProcID(arg.ProcId)

	if arg.Args != nil {
		for _, i := range arg.Args.Items() {
			message.SetParameter(arg.SdID, i.Key, utils.ToString(i.Value))
		}
	}

	out, err := message.String()
	if err != nil {
		scope.Log("rsyslog: %v", err)
	}

	err = logger.Write(out)
	if err != nil {
		scope.Log("rsyslog: %v", err)
	}
	return true
}

func (self RsyslogFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "rsyslog",
		Doc:      "Send an RFC5424 compliant remote syslog message.",
		ArgType:  type_map.AddType(scope, &RsyslogFunctionArgs{}),
		Version:  2,
		Metadata: vql.VQLMetadata().Permissions(acls.NETWORK).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RsyslogFunction{})
}
