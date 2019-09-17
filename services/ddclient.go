package services

// Based on github.com/clayshek/google-ddns-updater

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	ddns_service = "domains.google.com"
)

type DynDNSService struct {
	Done chan bool
	wg   sync.WaitGroup
}

func (self *DynDNSService) updateIP(config_obj *config_proto.Config) {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Checking DNS")

	externalIP, err := GetExternalIp()
	if err != nil {
		logger.Error("Unable to get external IP: %v", err)
		return
	}

	ddns_hostname := config_obj.Frontend.DynDns.Hostname
	hostnameIPs, err := GetCurrentDDNSIp(ddns_hostname)
	if err != nil {
		logger.Error("Unable to resolve DDNS hostname IP: %v", err)
		return
	}

	for _, ip := range hostnameIPs {
		if ip == externalIP {
			return
		}
	}

	logger.Info("DNS UPDATE REQUIRED. External IP=%v. %v=%v.",
		externalIP, ddns_hostname, hostnameIPs)

	reqstr := fmt.Sprintf(
		"https://%v/nic/update?hostname=%v&myip=%v",
		ddns_service,
		ddns_hostname,
		externalIP)
	logger.Debug("Submitting update request to %v", reqstr)

	err = UpdateDDNSRecord(
		config_obj,
		reqstr,
		config_obj.Frontend.DynDns.DdnsUsername,
		config_obj.Frontend.DynDns.DdnsPassword)
	if err != nil {
		logger.Error("Failed to update: %v", err)
		return
	}
}

func (self *DynDNSService) Start(config_obj *config_proto.Config) {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting the DynDNS service: Updating hostname %v",
		config_obj.Frontend.DynDns.Hostname)

	defer self.wg.Done()

	min_update_wait := config_obj.Frontend.DynDns.Frequency
	if min_update_wait == 0 {
		min_update_wait = 60
	}

	// First time check immediately.
	self.updateIP(config_obj)

	for {
		select {
		case <-self.Done:
			return

			// Do not try to update sooner than this or we
			// get banned. It takes a while for dns
			// records to propagate.
		case <-time.After(time.Duration(min_update_wait) * time.Second):
			self.updateIP(config_obj)
		}
	}
}

func (self *DynDNSService) Close() {
	close(self.Done)

	self.wg.Wait()
}

func startDynDNSService(config_obj *config_proto.Config) (*DynDNSService, error) {
	result := &DynDNSService{
		Done: make(chan bool),
	}

	if config_obj.Frontend.DynDns == nil ||
		config_obj.Frontend.DynDns.Hostname == "" {
		return result, nil
	}

	result.wg.Add(1)
	go result.Start(config_obj)

	return result, nil
}

func GetExternalIp() (string, error) {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return "Unable to determine external IP: %v ", err
	}
	defer resp.Body.Close()
	ip, err := ioutil.ReadAll(resp.Body)
	result := strings.TrimSpace(string(ip))

	if err != nil && err != io.EOF {
		return result, err
	}

	return result, nil
}

func GoogleDNSDialer(ctx context.Context, network, address string) (net.Conn, error) {
	d := net.Dialer{}
	return d.DialContext(ctx, "udp", "8.8.8.8:53")
}

func GetCurrentDDNSIp(fqdn string) ([]string, error) {
	r := net.Resolver{
		PreferGo: true,
		Dial:     GoogleDNSDialer,
	}
	ctx := context.Background()
	ips, err := r.LookupHost(ctx, fqdn)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

func UpdateDDNSRecord(config_obj *config_proto.Config,
	url, user, pw string) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	client := &http.Client{
		CheckRedirect: nil,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pw)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	logger.Debug("Update response: %v", string(body))

	return nil
}
