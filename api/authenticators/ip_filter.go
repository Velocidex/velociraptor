package authenticators

import (
	"net"
	"net/http"
	"strings"

	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Implement a src IP filter if required. This adds an additional
// layer of protection on the GUI.
func IpFilter(config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	if config_obj.GUI == nil || len(config_obj.GUI.AllowedCidr) == 0 {
		return api_utils.HandlerFunc(parent, parent.ServeHTTP)
	}

	ranges := []*net.IPNet{}
	for _, cidr := range config_obj.GUI.AllowedCidr {
		_, cidr_net, err := net.ParseCIDR(cidr)
		if err != nil {
			// Should never happen because sanity service should check
			// it already.
			panic("Invalid CIDR Range " + cidr)
		}
		ranges = append(ranges, cidr_net)
	}

	return api_utils.HandlerFunc(parent,
		func(w http.ResponseWriter, r *http.Request) {

			// If the user specified a forwarded header and the header is
			// there we must check it.
			if config_obj.GUI.ForwardedProxyHeader != "" {
				address_string := r.Header.Get(config_obj.GUI.ForwardedProxyHeader)
				ips := strings.Split(address_string, ", ")
				if len(ips) > 0 {
					// CIDR matched allow it.
					if matchCidr(ranges, ips...) {
						parent.ServeHTTP(w, r)
						return
					}
					http.Error(w, "rejected", http.StatusUnauthorized)
					return
				}
			}

			// Try to check the remote address now.
			remote_address := strings.Split(r.RemoteAddr, ":")[0]
			if matchCidr(ranges, remote_address) {
				parent.ServeHTTP(w, r)
				return
			}
			http.Error(w, "rejected", http.StatusUnauthorized)
		})
}

func matchCidr(ranges []*net.IPNet, ip_strings ...string) bool {
	for _, ip_str := range ip_strings {
		ip := net.ParseIP(ip_str)
		if ip == nil {
			return false
		}

		for _, cidr := range ranges {
			if cidr.Contains(ip) {
				return true
			}
		}
	}
	return false
}
