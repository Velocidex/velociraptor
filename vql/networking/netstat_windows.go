//go:build windows
// +build windows

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package networking

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// Function reference.
// https://docs.microsoft.com/en-us/windows/desktop/api/iphlpapi/nf-iphlpapi-getextendedtcptable
const (
	iphlpapiDll = "iphlpapi.dll"
	tcpFn       = "GetExtendedTcpTable"
	udpFn       = "GetExtendedUdpTable"

	// https://docs.microsoft.com/en-us/windows/desktop/api/iprtrmib/ne-iprtrmib-_tcp_table_class
	TCP_TABLE_OWNER_MODULE_ALL = 8
	UDP_TABLE_OWNER_MODULE     = 2
	tcpTableOwnerPidAll        = 5
)

var (
	MIB_TCP_STATE = map[int]string{
		1:  "CLOSED",
		2:  "LISTEN",
		3:  "SYN_SENT",
		4:  "SYN_RCVD",
		5:  "ESTAB",
		6:  "FIN_WAIT1",
		7:  "FIN_WAIT2",
		8:  "CLOSE_WAIT",
		9:  "CLOSING",
		10: "LAST_ACK",
		11: "TIME_WAIT",
		12: "DELETE_TCB",
	}
)

func (self *ConnectionStat) FamilyString() string {
	switch self.Family {
	case windows.AF_INET:
		return "IPv4"
	case windows.AF_INET6:
		return "IPv6"
	default:
		return fmt.Sprintf("%d", self.Family)
	}
}

func (self *ConnectionStat) TypeString() string {
	switch self.Type {
	case windows.SOCK_STREAM:
		return "TCP"
	case windows.SOCK_DGRAM:
		return "UDP"
	default:
		return fmt.Sprintf("%d", self.Type)
	}
}

func (self *ConnectionStat) Timestamp() time.Time {
	return self.timestamp
}

func runNetstat(
	ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("netstat: %s", err)
		return result
	}

	arg := &NetstatArgs{}

	logerror := func(err error) []vfilter.Row {
		scope.Log("netstat: %s", err.Error())
		return result
	}

	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		return logerror(err)
	}

	moduleHandle, err := windows.LoadLibrary(iphlpapiDll)
	if err != nil {
		return logerror(err)
	}

	GetExtendedTcpTable, err := windows.GetProcAddress(moduleHandle, "GetExtendedTcpTable")
	if err != nil {
		return logerror(err)
	}

	res, err := getNetTable(GetExtendedTcpTable, windows.AF_INET, TCP_TABLE_OWNER_MODULE_ALL)
	if err != nil {
		return logerror(err)
	}

	for _, item := range parse_MIB_TCPROW_OWNER_MODULE(res) {
		result = append(result, item)
	}

	res, err = getNetTable(GetExtendedTcpTable, windows.AF_INET6, TCP_TABLE_OWNER_MODULE_ALL)
	if err != nil {
		return logerror(err)
	}

	for _, item := range parse_MIB_TCP6ROW_OWNER_MODULE(res) {
		result = append(result, item)
	}

	GetExtendedUdpTable, err := windows.GetProcAddress(moduleHandle, "GetExtendedUdpTable")
	if err != nil {
		return logerror(err)
	}

	res, err = getNetTable(GetExtendedUdpTable, windows.AF_INET, UDP_TABLE_OWNER_MODULE)
	if err != nil {
		return logerror(err)
	}

	for _, item := range parse_MIB_UDPROW_OWNER_MODULE(res) {
		result = append(result, item)
	}

	res, err = getNetTable(GetExtendedUdpTable, windows.AF_INET6, UDP_TABLE_OWNER_MODULE)
	if err != nil {
		return logerror(err)
	}

	for _, item := range parse_MIB_UDP6ROW_OWNER_MODULE(res) {
		result = append(result, item)
	}

	return result
}

// https://docs.microsoft.com/en-au/previous-versions/windows/desktop/api/udpmib/ns-udpmib-_mib_udprow_owner_module
func parse_MIB_UDPROW_OWNER_MODULE(res []byte) []*ConnectionStat {
	result := []*ConnectionStat{}

	if res != nil && len(res) >= 4 {
		count := int(binary.LittleEndian.Uint32(res[0:4]))

		pos := 8 // Table starts 8 bytes in due to alignments.
		for n := 0; pos <= len(res) && n < count; n++ {
			timestamp := binary.LittleEndian.Uint64(
				res[pos+16:pos+24])/10000000 - 11644473600
			result = append(result, &ConnectionStat{
				Family: uint32(windows.AF_INET),
				Type:   windows.SOCK_DGRAM,
				Laddr: Addr{
					IP: net.IP(res[pos+0 : pos+4]).String(),
					Port: uint32(binary.BigEndian.Uint16(
						res[pos+4 : pos+8])),
				},
				Pid: int32(binary.LittleEndian.Uint32(
					res[pos+8 : pos+12])),
				timestamp: time.Unix(int64(timestamp), 0),
			})

			// struct length
			pos += 32 + 16*8
		}
	}

	return result
}

func parse_MIB_TCPROW_OWNER_MODULE(res []byte) []*ConnectionStat {
	result := []*ConnectionStat{}

	if res != nil && len(res) >= 4 {
		count := int(binary.LittleEndian.Uint32(res[0:4]))

		pos := 8 // Table starts 8 bytes in due to alignments.
		for n := 0; pos <= len(res) && n < count; n++ {
			state := int(binary.LittleEndian.Uint32(
				res[pos : pos+4]))
			status, pres := MIB_TCP_STATE[state]
			if !pres {
				status = fmt.Sprintf("%d", state)
			}

			timestamp := binary.LittleEndian.Uint64(
				res[pos+24:pos+32])/10000000 - 11644473600
			result = append(result, &ConnectionStat{
				Family: uint32(windows.AF_INET),
				Type:   1,
				Status: status,
				Laddr: Addr{
					IP: net.IP(res[pos+4 : pos+8]).String(),
					Port: uint32(binary.BigEndian.Uint16(
						res[pos+8 : pos+10])),
				},
				Raddr: Addr{
					IP: net.IP(res[pos+12 : pos+16]).String(),
					Port: uint32(binary.BigEndian.Uint16(
						res[pos+16 : pos+18])),
				},
				Pid: int32(binary.LittleEndian.Uint32(
					res[pos+20 : pos+24])),
				timestamp: time.Unix(int64(timestamp), 0),
			})

			// struct length
			pos += 32 + 16*8
		}
	}

	return result
}

func parse_MIB_TCP6ROW_OWNER_MODULE(res []byte) []*ConnectionStat {
	result := []*ConnectionStat{}

	if res != nil && len(res) >= 4 {
		count := int(binary.LittleEndian.Uint32(res[0:4]))

		pos := 8 // Table starts 8 bytes in due to alignments.
		for n := 0; pos <= len(res) && n < count; n++ {
			state := int(binary.LittleEndian.Uint32(
				res[pos+48 : pos+52]))
			status, pres := MIB_TCP_STATE[state]
			if !pres {
				status = fmt.Sprintf("%d", state)
			}

			timestamp := binary.LittleEndian.Uint64(
				res[pos+56:pos+64])/10000000 - 11644473600
			result = append(result, &ConnectionStat{
				Family: uint32(windows.AF_INET6),
				Type:   1,
				Status: status,
				Laddr: Addr{
					IP: net.IP(res[pos : pos+16]).String(),
					Port: uint32(binary.BigEndian.Uint16(
						res[pos+20 : pos+24])),
				},
				Raddr: Addr{
					IP: net.IP(res[pos+24 : pos+40]).String(),
					Port: uint32(binary.BigEndian.Uint16(
						res[pos+44 : pos+48])),
				},
				Pid: int32(binary.LittleEndian.Uint32(
					res[pos+52 : pos+56])),
				timestamp: time.Unix(int64(timestamp), 0),
			})

			// struct length
			pos += 64 + 16*8
		}
	}

	return result
}

// https://docs.microsoft.com/en-au/previous-versions/windows/desktop/api/udpmib/ns-udpmib-_mib_udprow_owner_module
func parse_MIB_UDP6ROW_OWNER_MODULE(res []byte) []*ConnectionStat {
	result := []*ConnectionStat{}

	if res != nil && len(res) >= 4 {
		count := int(binary.LittleEndian.Uint32(res[0:4]))

		pos := 8 // Table starts 8 bytes in due to alignments.
		for n := 0; pos <= len(res) && n < count; n++ {
			// Timestamp is aligned on 8 bytes
			timestamp := binary.LittleEndian.Uint64(
				res[pos+32:pos+40])/10000000 - 11644473600
			result = append(result, &ConnectionStat{
				Family: uint32(windows.AF_INET6),
				Type:   windows.SOCK_DGRAM,
				Laddr: Addr{
					IP: net.IP(res[pos+0 : pos+16]).String(),
					Port: uint32(binary.BigEndian.Uint16(
						res[pos+20 : pos+24])),
				},
				Pid: int32(binary.LittleEndian.Uint32(
					res[pos+24 : pos+28])),
				timestamp: time.Unix(int64(timestamp), 0),
			})

			// struct length
			pos += 48 + 16*8
		}
	}

	return result
}

// https://gist.github.com/adriansr/d2c1201be6bf2c2037e40d1306d72989
func getNetTable(fn uintptr, family int, class int) ([]byte, error) {
	var sorted uintptr
	size := uint32(8)
	ptr := []byte(nil)
	addr := uintptr(0)
	for {
		err, _, _ := syscall.Syscall6(
			fn, 5, addr,
			uintptr(unsafe.Pointer(&size)),
			sorted, uintptr(family), uintptr(class), 0)
		if err == 0 {
			return ptr, nil
		} else if err == uintptr(syscall.ERROR_INSUFFICIENT_BUFFER) {
			// realloc is needed.
			ptr = utils.AllocateBuff(int(size))
			addr = uintptr(unsafe.Pointer(&ptr[0]))
		} else {
			return nil, fmt.Errorf("getNetTable: %w", syscall.GetLastError())
		}
	}
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
