//go:build freebsd && cgo
// +build freebsd,cgo

package networking

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

/*
#include <string.h>

#include <sys/types.h>

#include <sys/socket.h>
#include <netinet/in.h>

#include <sys/socketvar.h>
#include <netinet/in_pcb.h>
#include <netinet/tcp_var.h>


typedef struct conn_stat {
   uint32_t family;
   uint32_t typ;
   uint16_t fport;
   uint16_t lport;
   char faddr[16];
   char laddr[16];
   uint32_t st;
} conn_stat;

typedef struct xinpgen xinpgen;
typedef struct xinpcb xinpcb;
typedef struct xtcpcb xtcpcb;

uint32_t extract_xtcpcb(xtcpcb* xt, conn_stat* cs) {
    if (xt->xt_inp.inp_vflag & INP_IPV6) {
        cs->family = AF_INET6;
    } else if (xt->xt_inp.inp_vflag & INP_IPV4) {
        cs->family = AF_INET;
    }
    cs->typ = SOCK_STREAM; // because TCP
    cs->st = xt->t_state;
    cs->fport = htons(xt->xt_inp.inp_inc.inc_ie.ie_fport);
    cs->lport = htons(xt->xt_inp.inp_inc.inc_ie.ie_lport);
    memcpy(cs->faddr, &(xt->xt_inp.inp_inc.inc_ie.ie_dependfaddr), 16);
    memcpy(cs->laddr, &(xt->xt_inp.inp_inc.inc_ie.ie_dependfaddr), 16);
    return xt->xt_len;
}

uint32_t extract_xinpcb(xinpcb* xi, conn_stat* cs) {
    if (xi->inp_vflag & INP_IPV6) {
        cs->family = AF_INET6;
    } else if (xi->inp_vflag & INP_IPV4) {
        cs->family = AF_INET;
    }
    cs->typ = SOCK_DGRAM; // because UDP
    cs->fport = htons(xi->inp_inc.inc_ie.ie_fport);
    cs->lport = htons(xi->inp_inc.inc_ie.ie_lport);
    memcpy(cs->faddr, &(xi->inp_inc.inc_ie.ie_dependfaddr), 16);
    memcpy(cs->laddr, &(xi->inp_inc.inc_ie.ie_dependfaddr), 16);
    return xi->xi_len;
}

*/
import "C"

func to_addr(family C.uint32_t, addr *[16]byte, port C.uint16_t) Addr {
	switch family {
	case C.AF_INET:
		return Addr{IP: net.IP(addr[12:]).String(), Port: uint32(port)}
	case C.AF_INET6:
		return Addr{IP: net.IP(addr[:]).String(), Port: uint32(port)}
	default:
		// This should not happen.
		return Addr{Port: uint32(port)}
	}
}

// constants from xnu/bsd/netinet/tcp_fsm.h, renamed to match Windows
// implementation
var socketStates = map[uint16]string{
	0:  "CLOSE", // CLOSED
	1:  "LISTEN",
	2:  "SYN_SENT",
	3:  "SYN_RCVD", // SYN_RECEIVED
	4:  "ESTAB",    // ESTABLISHED
	5:  "CLOSE_WAIT",
	6:  "FIN_WAIT1", // FIN_WAIT_1
	7:  "CLOSING",
	8:  "LAST_ACK",
	9:  "FIN_WAIT2", // FIN_WAIT_2
	10: "TIME_WAIT",
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

func gatherTCP() ([]*ConnectionStat, error) {
	b, err := syscall.Sysctl("net.inet.tcp.pcblist")
	if err != nil {
		return nil, err
	}
	buf := []byte(b)
	// syscall.Sysctl seems to expect null-terminated strings.
	buf = append(buf, 0)
	if len(buf) < int(unsafe.Sizeof(C.xinpgen{})) {
		return nil, errors.New("buffer too short")
	}
	xig := (*C.xinpgen)(unsafe.Pointer(&buf[0]))
	var rv []*ConnectionStat

	buf = buf[int(unsafe.Sizeof(C.xinpgen{})):]
	for n := 0; n < int(xig.xig_count); n += 1 {
		if len(buf) < int(unsafe.Sizeof(C.xtcpcb{})) {
			return nil, fmt.Errorf("unexpected end of buffer after %d records, %d bytes left",
				n, len(buf))
		}
		var cs C.conn_stat
		len := C.extract_xtcpcb((*C.xtcpcb)(unsafe.Pointer(&buf[0])), &cs)
		status := socketStates[uint16(cs.st)]
		rv = append(rv, &ConnectionStat{
			Family: uint32(cs.family),
			Type:   uint32(cs.typ),
			Status: status,
			Laddr:  to_addr(cs.family, (*[16]byte)(unsafe.Pointer(&cs.laddr[0])), cs.lport),
			Raddr:  to_addr(cs.family, (*[16]byte)(unsafe.Pointer(&cs.faddr[0])), cs.fport),
		})
		buf = buf[int(len):]
	}
	if len(buf) < int(unsafe.Sizeof(C.xinpgen{})) {
		return nil, errors.New("unexpected end of buffer while reading suffix xnipgen")
	}

	return rv, nil
}

func gatherUDP() ([]*ConnectionStat, error) {
	b, err := syscall.Sysctl("net.inet.udp.pcblist")
	if err != nil {
		return nil, err
	}
	buf := []byte(b)
	// syscall.Sysctl seems to expect null-terminated strings.
	buf = append(buf, 0)
	if len(buf) < int(unsafe.Sizeof(C.xinpgen{})) {
		return nil, errors.New("buffer too short")
	}
	xig := (*C.xinpgen)(unsafe.Pointer(&buf[0]))
	var rv []*ConnectionStat

	buf = buf[int(unsafe.Sizeof(C.xinpgen{})):]
	for n := 0; n < int(xig.xig_count); n += 1 {
		if len(buf) < int(unsafe.Sizeof(C.xinpcb{})) {
			return nil, fmt.Errorf("unexpected end of buffer after %d record, %d bytes left",
				n, len(buf))
		}
		var cs C.conn_stat
		len := C.extract_xinpcb((*C.xinpcb)(unsafe.Pointer(&buf[0])), &cs)
		rv = append(rv, &ConnectionStat{
			Family: uint32(cs.family),
			Type:   uint32(cs.typ),
			Laddr:  to_addr(cs.family, (*[16]byte)(unsafe.Pointer(&cs.laddr[0])), cs.lport),
			Raddr:  to_addr(cs.family, (*[16]byte)(unsafe.Pointer(&cs.faddr[0])), cs.fport),
		})
		buf = buf[int(len):]
	}
	if len(buf) < int(unsafe.Sizeof(C.xinpgen{})) {
		return nil, errors.New("unexpected end of buffer while reading suffix xnipgen")
	}

	return rv, nil
}

func runNetstat(ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row
	if cs, err := gatherTCP(); err != nil {
		scope.Log("netstat: gatherTCP: %v", err)
	} else {
		for _, item := range cs {
			result = append(result, item)
		}
	}
	if cs, err := gatherUDP(); err != nil {
		scope.Log("netstat: gatherUDP: %v", err)
	} else {
		for _, item := range cs {
			result = append(result, item)
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
