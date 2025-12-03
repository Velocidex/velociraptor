//go:build darwin && cgo
// +build darwin,cgo

/*
  This is an implementation of netstat().

  Reference for this code is
  https://opensource.apple.com/source/network_cmds/network_cmds-307/netstat.tproj/inet.c
  and function protopr()

*/

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

#include <arpa/inet.h>
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
    } else if (xt->xt_inp.inp_vflag & INP_V4MAPPEDV6) {
        cs->family = AF_INET6;
    } else if (xt->xt_inp.inp_vflag & INP_IPV4) {
        cs->family = AF_INET;
    }
    cs->typ = SOCK_STREAM; // because TCP
    cs->st = xt->xt_tp.t_state;
    cs->fport = htons(xt->xt_inp.inp_fport);
    cs->lport = htons(xt->xt_inp.inp_lport);
    memcpy(cs->faddr, &(xt->xt_inp.inp_dependfaddr), 16);
    memcpy(cs->laddr, &(xt->xt_inp.inp_dependladdr), 16);
    return xt->xt_len;
}

uint32_t extract_xinpcb(xinpcb* xi, conn_stat* cs) {
    if (xi->xi_inp.inp_vflag & INP_IPV6) {
        cs->family = AF_INET6;
    } else if (xi->xi_inp.inp_vflag & INP_V4MAPPEDV6) {
        cs->family = AF_INET6;
    } else if (xi->xi_inp.inp_vflag & INP_IPV4) {
        cs->family = AF_INET;
    }
    cs->typ = SOCK_DGRAM; // because UDP
    cs->fport = htons(xi->xi_inp.inp_fport);
    cs->lport = htons(xi->xi_inp.inp_lport);
    memcpy(cs->faddr, &(xi->xi_inp.inp_dependfaddr), 16);
    memcpy(cs->laddr, &(xi->xi_inp.inp_dependladdr), 16);
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

// Walk over all the entries and call the callback on each.
func walkEntries(sysctl string, cb func(buff []byte)) error {
	b, err := syscall.Sysctl(sysctl)
	if err != nil {
		return err
	}
	buf := []byte(b)

	sizeof_xinpgen := int(unsafe.Sizeof(C.xinpgen{}))

	// Need to have at least one xinpgen to process.
	if len(buf) < sizeof_xinpgen {
		return errors.New("buffer too short")
	}

	offset := 0

	xig := (*C.xinpgen)(unsafe.Pointer(&buf[offset]))
	total_count := int(xig.xig_count)

	// Now we have a packed list of xinpgen
	for n := 0; n < total_count; n += 1 {
		// Not enough space for another record.
		if offset+sizeof_xinpgen > len(buf) {
			break
		}

		xig := (*C.xinpgen)(unsafe.Pointer(&buf[offset]))
		len := int(xig.xig_len)

		// We need to make some progress.
		if len < int(sizeof_xinpgen) {
			break
		}

		cb(buf[offset:])

		// Go to the next one
		offset += len
	}

	return nil
}

func gatherTCP() ([]*ConnectionStat, error) {
	res := []*ConnectionStat{}

	sizeof_xtcpcb := int(unsafe.Sizeof(C.xtcpcb{}))

	err := walkEntries("net.inet.tcp.pcblist",
		func(buf []byte) {
			xig := (*C.xinpgen)(unsafe.Pointer(&buf[0]))

			// Buffer is not large enough for a xtcpcb
			if int(xig.xig_len) < sizeof_xtcpcb {
				return
			}

			// Read the record.
			xtcpcb := (*C.xtcpcb)(unsafe.Pointer(&buf[0]))

			var cs C.conn_stat

			C.extract_xtcpcb(xtcpcb, &cs)

			status := socketStates[uint16(cs.st)]
			conn_stat := &ConnectionStat{
				Family: uint32(cs.family),
				Type:   uint32(cs.typ),
				Status: status,
				Laddr:  to_addr(cs.family, (*[16]byte)(unsafe.Pointer(&cs.laddr[0])), cs.lport),
				Raddr:  to_addr(cs.family, (*[16]byte)(unsafe.Pointer(&cs.faddr[0])), cs.fport),
			}
			res = append(res, conn_stat)
		})
	return res, err
}

func gatherUDP() ([]*ConnectionStat, error) {
	res := []*ConnectionStat{}

	sizeof_xinpcb := int(unsafe.Sizeof(C.xinpcb{}))
	err := walkEntries("net.inet.udp.pcblist",
		func(buf []byte) {
			xig := (*C.xinpgen)(unsafe.Pointer(&buf[0]))

			// Buffer is not large enough for a xtcpcb
			if int(xig.xig_len) < sizeof_xinpcb {
				return
			}

			// Read the record.
			xinpcb := (*C.xinpcb)(unsafe.Pointer(&buf[0]))

			var cs C.conn_stat

			C.extract_xinpcb(xinpcb, &cs)
			conn_stat := &ConnectionStat{
				Family: uint32(cs.family),
				Type:   uint32(cs.typ),
				Laddr:  to_addr(cs.family, (*[16]byte)(unsafe.Pointer(&cs.laddr[0])), cs.lport),
				Raddr:  to_addr(cs.family, (*[16]byte)(unsafe.Pointer(&cs.faddr[0])), cs.fport),
			}
			res = append(res, conn_stat)
		})
	return res, err
}

func runNetstat(ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row

	cs, err := gatherTCP()
	if err != nil {
		scope.Log("netstat: gatherTCP: %v", err)
	}

	for _, item := range cs {
		result = append(result, item)
	}

	cs, err = gatherUDP()
	if err != nil {
		scope.Log("netstat: gatherUDP: %v", err)
	}

	for _, item := range cs {
		result = append(result, item)
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
