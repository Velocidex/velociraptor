package networking

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type NetcatPluginArgs struct {
	Address     string `vfilter:"required,field=address,doc=The address to connect to (can be a file in case of a unix domain socket)"`
	AddressType string `vfilter:"optional,field=type,doc=Can be tcp or unix (default TCP)"`
	Send        string `vfilter:"optional,field=send,doc=Data to send before reading"`
	Sep         string `vfilter:"optional,field=sep,doc=The separator that will be used to split (default - line feed)"`
	Chunk       int    `vfilter:"optional,field=chunk_size,doc=Read input with this chunk size (default 64kb)"`
	Retry       int    `vfilter:"optional,field=retry,doc=Seconds to wait before retry - default 0 - do not retry"`
}

type NetcatPlugin struct{}

func (self *NetcatPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "netcat", args)()

		arg := &NetcatPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("netcat: %s", err)
			return
		}

		err = vql_subsystem.CheckAccess(scope, acls.NETWORK)
		if err != nil {
			scope.Log("netcat: %s", err)
			return
		}

		for {
			self.connectOnce(ctx, scope, arg, output_chan)
			if arg.Retry == 0 {
				break
			}
			time.Sleep(time.Duration(arg.Retry) * time.Second)
		}

	}()

	return output_chan
}

func (self NetcatPlugin) connectOnce(
	ctx context.Context, scope vfilter.Scope, arg *NetcatPluginArgs,
	output_chan chan vfilter.Row) {
	socket_type := arg.AddressType
	switch socket_type {
	case "":
		socket_type = "tcp"
	case "tcp", "unix", "udp":
	default:
		scope.Log("netcat: unsupported address type (%v)", arg.AddressType)
		return
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, socket_type, arg.Address)
	if err != nil {
		scope.Log("netcat: %s", err)
		return
	}
	defer conn.Close()

	if arg.Send != "" {
		go func() {
			_, err := conn.Write([]byte(arg.Send))
			if err != nil {
				scope.Log("netcat: %s", err)
			}
		}()
	}

	sep := arg.Sep
	if sep == "" {
		sep = "\n"
	}

	chunk_size := arg.Chunk
	if chunk_size == 0 {
		chunk_size = 64 * 1024
	}

	if chunk_size > 10*1024*1024 {
		chunk_size = 10 * 1024 * 1024
	}

	buf := make([]byte, chunk_size)
	offset := 0
	for {
		n, err := conn.Read(buf[offset:])
		if err != nil {
			return
		}

		lines := strings.Split(string(buf[:offset+n]), sep)
		for idx, line := range lines {
			if idx == len(lines)-1 {
				// If last line is non empty the buffer does not
				// end with a separator, copy the last line to the
				// old buffer and start reading from there.
				last_line := lines[idx]
				if last_line != "" {
					copy(buf, []byte(last_line))
					offset = len(last_line)
				}
				continue
			}

			if line == "" {
				continue
			}

			select {
			case <-ctx.Done():
				return

			case output_chan <- ordereddict.NewDict().Set("Data", line):
			}
		}
	}
}

func (self NetcatPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "netcat",
		Doc:      "Make a tcp connection and read data from a socket.",
		ArgType:  type_map.AddType(scope, &NetcatPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.NETWORK).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&NetcatPlugin{})
}
