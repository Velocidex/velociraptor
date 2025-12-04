//go:build linux
// +build linux

package networking

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

// constants from netinet/tcp.h, renamed to match Windows
// implementation
var socketStates = map[uint16]string{
	1:  "ESTAB", // ESTABLISHED
	2:  "SYN_SENT",
	3:  "SYN_RCVD", // SYN_RECV
	4:  "FIN_WAIT1",
	5:  "FIN_WAIT2",
	6:  "TIME_WAIT",
	7:  "CLOSE",
	8:  "CLOSE_WAIT",
	9:  "LAST_ACK",
	10: "LISTEN",
	11: "CLOSING",
}

var netConstants = map[string][2]uint32{
	"tcp":  {syscall.AF_INET, syscall.SOCK_STREAM},
	"tcp6": {syscall.AF_INET6, syscall.SOCK_STREAM},
	"udp":  {syscall.AF_INET, syscall.SOCK_DGRAM},
	"udp6": {syscall.AF_INET6, syscall.SOCK_DGRAM},
}

func (self *ConnectionStat) FamilyString() string {
	switch self.Family {
	case syscall.AF_INET:
		return "IPv4"
	case syscall.AF_INET6:
		return "IPv6"
	default:
		return fmt.Sprintf("%d", self.Family)
	}
}

func (self *ConnectionStat) TypeString() string {
	switch self.Type {
	case syscall.SOCK_STREAM:
		return "TCP"
	case syscall.SOCK_DGRAM:
		return "UDP"
	default:
		return fmt.Sprintf("%d", self.Type)
	}
}

func (self *ConnectionStat) Timestamp() time.Time {
	return self.timestamp
}

// parse addressses
// 0100007F:0050 -> 127.0.0.1 : 80
// 00000000000000000000000001000000:0050 -> ::1 : 80
func parseIPPort(addr string) (net.IP, uint32, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return nil, 0, fmt.Errorf("Can't parse address entry '%s'", addr)
	}
	var ip net.IP
	switch len(parts[0]) {
	case 8:
		ip = make(net.IP, net.IPv4len)
		u, err := strconv.ParseUint(parts[0], 16, 32)
		if err != nil {
			return nil, 0, err
		}
		binary.LittleEndian.PutUint32(ip[:], uint32(u))
	case 32:
		ip = make(net.IP, net.IPv6len)
		for i := 0; i < 4; i += 1 {
			u, err := strconv.ParseUint(parts[0][8*i:8*i+8], 16, 32)
			if err != nil {
				return nil, 0, err
			}
			binary.LittleEndian.PutUint32(ip[4*i:4*i+4], uint32(u))
		}
	default:
		return nil, 0, fmt.Errorf("unrecognized address length %d", len(parts[0]))
	}
	port, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		return nil, 0, err
	}
	return ip, uint32(port), nil
}

type socketInfo map[uint64]int32

// Build socket inode -> PID mapping
func gatherSocketInfo() (socketInfo, error) {
	m := make(socketInfo)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fddir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fddir)
		if err != nil {
			continue
		}
		for _, fdfile := range fds {
			fdpath := path.Join(fddir, fdfile.Name())
			fi, err := os.Stat(fdpath)
			if err != nil {
				continue
			}
			lname, err := os.Readlink(fdpath)
			if err != nil || !strings.HasPrefix(lname, "socket:[") {
				continue
			}
			st, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				continue
			}
			if lname != fmt.Sprintf("socket:[%d]", st.Ino) {
				continue
			}
			m[st.Ino] = int32(pid)
		}
	}
	return m, nil
}

func readProcNet(which string, si socketInfo) ([]*ConnectionStat, error) {
	var addrFamily, addrType uint32
	if c, ok := netConstants[which]; !ok {
		return nil, fmt.Errorf("readProcNet: unrecognized type '%s'", which)
	} else {
		addrFamily = c[0]
		addrType = c[1]
	}

	f, err := os.Open("/proc/net/" + which)
	if err != nil {
		return nil, err
	}
	br := bufio.NewScanner(f)
	br.Scan() // skip header

	/*
		sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
		7:  0103000A:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 28123 1 00000000872a608c 100 0 0 10 5
	*/
	var rv []*ConnectionStat
	for br.Scan() {
		fields := strings.Fields(br.Text())
		if len(fields) < 10 {
			continue
		}
		lip, lport, err := parseIPPort(fields[1])
		if err != nil {
			continue
		}
		rip, rport, err := parseIPPort(fields[2])
		if err != nil {
			continue
		}
		u, err := strconv.ParseUint(fields[3], 16, 16)
		if err != nil {
			continue
		}
		st, ok := socketStates[uint16(u)]
		if !ok {
			continue
		}
		ino, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		pid, ok := si[ino]
		if !ok {
			pid = -1
		}
		rv = append(rv, &ConnectionStat{
			// Fd:     uint32,
			Family: addrFamily,
			Type:   addrType,
			Laddr:  Addr{lip.String(), lport},
			Raddr:  Addr{rip.String(), rport},
			Status: st,
			Pid:    pid,
			// timestamp: not available on Linux
		})
	}
	return rv, nil
}

func runNetstat(ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row
	si, err := gatherSocketInfo()
	scope.Log("netstat: gathered %d sockinfo entries, err=%v", len(si), err)
	for _, which := range [...]string{"tcp", "tcp6", "udp", "udp6"} {
		if cs, err := readProcNet(which, si); err != nil {
			scope.Log("netstat: parse %s: %s", which, err)
			continue
		} else {
			for _, item := range cs {
				result = append(result, item)
			}
		}
	}

	return result
}

var _Netstat = vfilter.GenericListPlugin{
	PluginName: "netstat",
	Doc:        "Collect network information.",
	Function:   runNetstat,
	ArgType:    &NetstatArgs{},
	Metadata:   vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
}

func init() {
	vql_subsystem.RegisterPlugin(&_Netstat)
}
