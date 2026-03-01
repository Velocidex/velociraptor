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
#include <libproc.h>
#include <sys/proc_info.h>

typedef struct conn_stat {
   uint32_t family;
   uint32_t typ;
   uint16_t fport;
   uint16_t lport;
   char faddr[16];
   char laddr[16];
   uint32_t st;
} conn_stat;

// Socket info extracted from proc_pidfdinfo for PID mapping.
typedef struct sock_pid_info {
   int32_t  pid;
   int32_t  family;
   int32_t  proto;
   uint16_t lport;
   uint16_t fport;
   char     laddr[16];
   char     faddr[16];
} sock_pid_info;

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

// Extract socket address info from a socket_fdinfo into a flat struct.
// Returns 1 on success (TCP or UDP inet socket), 0 otherwise.
int extract_socket_info(struct socket_fdinfo* sfi, sock_pid_info* out) {
    struct socket_info* si = &sfi->psi;

    if (si->soi_family != AF_INET && si->soi_family != AF_INET6) {
        return 0;
    }

    out->family = si->soi_family;
    out->proto = si->soi_protocol;

    struct in_sockinfo* ini;
    if (si->soi_protocol == IPPROTO_TCP) {
        ini = &si->soi_proto.pri_tcp.tcpsi_ini;
    } else if (si->soi_protocol == IPPROTO_UDP) {
        ini = &si->soi_proto.pri_in;
    } else {
        return 0;
    }

    out->lport = ntohs((uint16_t)ini->insi_lport);
    out->fport = ntohs((uint16_t)ini->insi_fport);

    if (si->soi_family == AF_INET) {
        memset(out->laddr, 0, 16);
        memcpy(out->laddr + 12, &ini->insi_laddr.ina_46.i46a_addr4.s_addr, 4);
        memset(out->faddr, 0, 16);
        memcpy(out->faddr + 12, &ini->insi_faddr.ina_46.i46a_addr4.s_addr, 4);
    } else {
        memcpy(out->laddr, &ini->insi_laddr.ina_6, 16);
        memcpy(out->faddr, &ini->insi_faddr.ina_6, 16);
    }

    return 1;
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

type socketKey struct {
	Family uint32
	Proto  uint32
	Laddr  string
	Lport  uint32
	Raddr  string
	Rport  uint32
}

// buildSocketPIDMap uses macOS libproc APIs to map sockets to PIDs.
func buildSocketPIDMap() map[socketKey]int32 {
	m := make(map[socketKey]int32)

	// Get the number of PIDs. proc_listpids returns bytes, not count.
	nbytesNeeded := int(C.proc_listpids(C.PROC_ALL_PIDS, 0, nil, 0))
	if nbytesNeeded <= 0 {
		return m
	}

	// Over-allocate by 20% to handle processes spawning between calls.
	npids := nbytesNeeded/int(unsafe.Sizeof(C.int(0)))*120/100 + 1
	pids := make([]C.int, npids)
	nbytes := C.proc_listpids(C.PROC_ALL_PIDS, 0,
		unsafe.Pointer(&pids[0]), C.int(npids*int(unsafe.Sizeof(pids[0]))))
	if nbytes <= 0 {
		return m
	}
	npids = int(nbytes) / int(unsafe.Sizeof(pids[0]))

	for i := 0; i < npids; i++ {
		pid := pids[i]
		if pid == 0 {
			continue
		}

		// Get FD list size, over-allocate for TOCTOU safety.
		fdBufSize := C.proc_pidinfo(pid, C.PROC_PIDLISTFDS, 0, nil, 0)
		if fdBufSize <= 0 {
			continue
		}

		nfds := int(fdBufSize)/int(C.PROC_PIDLISTFD_SIZE)*120/100 + 1
		fds := make([]C.struct_proc_fdinfo, nfds)
		fdBufSize = C.int(nfds) * C.int(C.PROC_PIDLISTFD_SIZE)
		fdBufSize = C.proc_pidinfo(pid, C.PROC_PIDLISTFDS, 0,
			unsafe.Pointer(&fds[0]), fdBufSize)
		if fdBufSize <= 0 {
			continue
		}
		nfds = int(fdBufSize) / int(C.PROC_PIDLISTFD_SIZE)

		for j := 0; j < nfds; j++ {
			if fds[j].proc_fdtype != C.PROX_FDTYPE_SOCKET {
				continue
			}

			var sfi C.struct_socket_fdinfo
			nb := C.proc_pidfdinfo(pid, fds[j].proc_fd,
				C.PROC_PIDFDSOCKETINFO,
				unsafe.Pointer(&sfi),
				C.int(unsafe.Sizeof(sfi)))
			if nb != C.int(unsafe.Sizeof(sfi)) {
				continue
			}

			var spi C.sock_pid_info
			spi.pid = C.int32_t(pid)
			if C.extract_socket_info(&sfi, &spi) == 0 {
				continue
			}

			family := uint32(spi.family)
			laddr := to_addr(C.uint32_t(spi.family),
				(*[16]byte)(unsafe.Pointer(&spi.laddr[0])),
				C.uint16_t(spi.lport))
			raddr := to_addr(C.uint32_t(spi.family),
				(*[16]byte)(unsafe.Pointer(&spi.faddr[0])),
				C.uint16_t(spi.fport))

			key := socketKey{
				Family: family,
				Proto:  uint32(spi.proto),
				Laddr:  laddr.IP,
				Lport:  laddr.Port,
				Raddr:  raddr.IP,
				Rport:  raddr.Port,
			}
			m[key] = int32(pid)

			// IPv6 sockets carrying IPv4 traffic may appear as
			// AF_INET in the sysctl data. Store an AF_INET key
			// too so the lookup succeeds either way.
			if family == syscall.AF_INET6 {
				lraw := (*[16]byte)(unsafe.Pointer(&spi.laddr[0]))
				rraw := (*[16]byte)(unsafe.Pointer(&spi.faddr[0]))
				lip4 := net.IP(lraw[:]).To4()
				rip4 := net.IP(rraw[:]).To4()
				if lip4 != nil && rip4 != nil {
					key4 := socketKey{
						Family: syscall.AF_INET,
						Proto:  uint32(spi.proto),
						Laddr:  lip4.String(),
						Lport:  laddr.Port,
						Raddr:  rip4.String(),
						Rport:  raddr.Port,
					}
					m[key4] = int32(pid)
				}
			}
		}
	}

	return m
}

func runNetstat(ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row

	pidMap := buildSocketPIDMap()

	cs, err := gatherTCP()
	if err != nil {
		scope.Log("netstat: gatherTCP: %v", err)
	}

	for _, item := range cs {
		key := socketKey{
			Family: item.Family,
			Proto:  syscall.IPPROTO_TCP,
			Laddr:  item.Laddr.IP,
			Lport:  item.Laddr.Port,
			Raddr:  item.Raddr.IP,
			Rport:  item.Raddr.Port,
		}
		if pid, ok := pidMap[key]; ok {
			item.Pid = pid
		}
		result = append(result, item)
	}

	cs, err = gatherUDP()
	if err != nil {
		scope.Log("netstat: gatherUDP: %v", err)
	}

	for _, item := range cs {
		key := socketKey{
			Family: item.Family,
			Proto:  syscall.IPPROTO_UDP,
			Laddr:  item.Laddr.IP,
			Lport:  item.Laddr.Port,
			Raddr:  item.Raddr.IP,
			Rport:  item.Raddr.Port,
		}
		if pid, ok := pidMap[key]; ok {
			item.Pid = pid
		}
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
