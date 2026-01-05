//go:build linux && (arm64 || amd64)
// +build linux
// +build arm64 amd64

package ebpf

import (
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/tracee_velociraptor/userspace/types/trace"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/tools/dns"
)

func parseDNSResponse(event *ordereddict.Dict) *ordereddict.Dict {
	dns_response_any := utils.GetAny(event, "EventData.dns_response")
	if dns_response_any != nil {
		dns_responses, ok := dns_response_any.([]trace.DnsResponseData)
		if ok {
			for _, resp := range dns_responses {
				for _, answer := range resp.DnsAnswer {
					switch answer.Type {
					// Forward lookup
					case "AAAA", "A":
						dns.DNSCache.Set(answer.Answer, resp.QueryData.Query)

					// Reverse lookup
					case "PTR":
						query := resp.QueryData.Query
						if strings.HasSuffix(query, "in-addr.arpa") {
							parts := strings.Split(query, ".")
							if len(parts) > 4 {
								ip := fmt.Sprintf("%v.%v.%v.%v",
									parts[3], parts[2], parts[1], parts[0])
								dns.DNSCache.Set(ip, answer.Answer)
							}
						}
					}

				}
			}
		}
	}

	return event
}

func enrichNetwork(event *ordereddict.Dict) *ordereddict.Dict {
	remote_addr_any := utils.GetAny(event, "EventData.remote_addr")
	if remote_addr_any != nil {
		remote_addr, ok := remote_addr_any.(map[string]string)
		if ok {
			sin_addr, pres := remote_addr["sin_addr"]
			if pres {
				names := dns.DNSCache.ByIP(sin_addr)
				remote_addr["dns_names"] = strings.Join(names, ", ")
			}
		}
	}

	return event
}

func enrich(event *ordereddict.Dict) *ordereddict.Dict {
	// Special post processing of events.
	event_type := utils.GetString(event, "System.EventName")
	switch event_type {

	// Parse DNS packets to cache answers
	case "net_packet_dns_response":
		return parseDNSResponse(event)

	case "security_socket_connect":
		return enrichNetwork(event)

	}
	return event
}
