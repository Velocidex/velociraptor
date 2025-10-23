package ddclient

// Based on github.com/clayshek/google-ddns-updater

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DynDNSService struct {
	config_obj *config_proto.Config

	external_ip_url string
	dns_server      string

	updater Updater
}

// Failing to update the DNS is not a fatal error we can try again
// later.
func (self *DynDNSService) updateIP(
	ctx context.Context, config_obj *config_proto.Config) {
	if config_obj.Frontend == nil || config_obj.Frontend.DynDns == nil {
		return
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("DynDns: Checking DNS with %v", self.external_ip_url)

	externalIP, err := self.GetExternalIp()
	if err != nil {
		logger.Error("DynDns: Unable to get external IP: %v", err)
		return
	}

	ddns_hostname := config_obj.Frontend.Hostname
	// If we can not resolve the current hostname then lets try to
	// update it anyway.
	hostnameIPs, _ := self.GetCurrentDDNSIp(ddns_hostname)
	for _, ip := range hostnameIPs {
		if ip == externalIP {
			return
		}
	}

	logger.Info("DynDns: DNS UPDATE REQUIRED. External IP=%v. %v=%v.",
		externalIP, ddns_hostname, hostnameIPs)

	err = self.updater.UpdateDDNSRecord(ctx, config_obj, externalIP)
	if err != nil {
		logger.Error("DynDns: Unable to set dns: %v", err)
		return
	}
}

func (self *DynDNSService) Start(
	ctx context.Context, config_obj *config_proto.Config) {

	if config_obj.Frontend == nil || config_obj.Frontend.DynDns == nil {
		return
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> the DynDNS service: Updating hostname %v with checkip URL %v",
		config_obj.Frontend.Hostname, self.external_ip_url)

	min_update_wait := config_obj.Frontend.DynDns.Frequency
	if min_update_wait == 0 {
		min_update_wait = 60
	}

	// First time check immediately.
	self.updateIP(ctx, config_obj)

	for {
		select {
		case <-ctx.Done():
			return

			// Do not try to update sooner than this or we
			// get banned. It takes a while for dns
			// records to propagate.
		case <-time.After(time.Duration(min_update_wait) * time.Second):
			self.updateIP(ctx, config_obj)
		}
	}
}

func StartDynDNSService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (err error) {

	if config_obj.Frontend == nil ||
		config_obj.Frontend.DynDns == nil ||
		config_obj.Frontend.Hostname == "" {
		return nil
	}

	result := &DynDNSService{
		config_obj:      config_obj,
		external_ip_url: config_obj.Frontend.DynDns.CheckipUrl,
		dns_server:      config_obj.Frontend.DynDns.DnsServer,
	}

	dyndns_type := strings.ToLower(config_obj.Frontend.DynDns.Type)
	if config_obj.Frontend.DynDns.Type == "" {
		if config_obj.Frontend.DynDns.DdnsUsername != "" {
			dyndns_type = "noip"
		} else {
			// No DynDNS specified. This backwards compatibility
			// setting ignores the dyndns setting when both the type
			// is unset and the ddns_username is not set. This allows
			// a setting like:
			// dyndns: {}
			//
			// To mean unconfigured dyndns service.
			return nil
		}
	}

	switch dyndns_type {
	case "noip":
		result.updater, err = NewNoIPUpdater(config_obj)

	case "cloudflare":
		result.updater, err = NewCloudflareUpdater(config_obj)

	case "":
		return nil

	default:
		return errors.New("DynDns: provider type not supported (currently only noip and cloudflare)")
	}
	if err != nil {
		return err
	}

	// Set sensible defaults that should work reliably most of the
	// time.
	if result.external_ip_url == "" {
		result.external_ip_url = "https://wtfismyip.com/text"
	}

	if result.dns_server == "" {
		result.dns_server = "8.8.8.8:53"
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		result.Start(ctx, config_obj)
	}()

	return nil
}

func (self *DynDNSService) GetExternalIp() (string, error) {
	resp, err := http.Get(self.external_ip_url)
	if err != nil {
		return "Unable to determine external IP: %v ", err
	}
	defer resp.Body.Close()
	ip, err := utils.ReadAllWithLimit(resp.Body, constants.MAX_MEMORY)
	result := strings.TrimSpace(string(ip))

	if err != nil && err != io.EOF {
		return result, err
	}

	return result, nil
}

func (self *DynDNSService) GetDNSDialer(ctx context.Context, network, address string) (net.Conn, error) {
	d := net.Dialer{}
	return d.DialContext(ctx, "udp", self.dns_server)
}

func (self *DynDNSService) GetCurrentDDNSIp(fqdn string) ([]string, error) {
	r := net.Resolver{
		PreferGo: true,
		Dial:     self.GetDNSDialer,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ips, err := r.LookupHost(ctx, fqdn)
	if err != nil {
		return nil, err
	}
	return ips, nil
}
