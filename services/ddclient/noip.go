package ddclient

/*
   This is an updater for no-ip.com - a free dynamic DNS provider.

1. Step one - sign into no-ip.com
2. Add a hostname to one of the free domain names or buy a domain name.
3. Generate a DDNS Key - this will generate a username and password

Sample config:
```yaml
Frontend:
  ...
  dyn_dns:
    type: noip
    ddns_username: USER123
    ddns_password: XXXYYYZZZ
```

This provider also supports other formats for the update URL so it can
be used with other dyndns providers. The `update_url` field is a
Golang template which will be expanded into the full URL.

For noip.com the template defaults to
https://dynupdate.no-ip.com/nic/update?hostname={{.Hostname}}&myip={{.IP}}


*/

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type _updateRecord struct {
	Hostname string
	IP       string
}

type NoIPUpdater struct {
	username, password, hostname, service string
	url_template                          *template.Template
}

func (self NoIPUpdater) UpdateDDNSRecord(
	ctx context.Context, config_obj *config_proto.Config,
	external_ip string) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	update_record := _updateRecord{
		Hostname: self.hostname,
		IP:       external_ip,
	}
	output := &bytes.Buffer{}
	err := self.url_template.Execute(output, update_record)
	if err != nil {
		return err
	}
	url := string(output.Bytes())
	logger.Debug("DynDns: Submitting update request to %v", url)

	client := &http.Client{
		CheckRedirect: nil,
		Transport: &http.Transport{
			Proxy: networking.GetProxy(),
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", constants.USER_AGENT)
	req.SetBasicAuth(self.username, self.password)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := utils.ReadAllWithLimit(req.Body, constants.MAX_MEMORY)
	if err != nil {
		return err
	}

	logger.Debug("DynDns: Update response: %v", string(body))

	return nil
}

func NewNoIPUpdater(config_obj *config_proto.Config) (Updater, error) {
	if config_obj.Frontend.DynDns.DdnsUsername == "" {
		return nil, errors.New("DynDns: Username must be provided for the NoIP service")
	}

	if config_obj.Frontend.DynDns.DdnsPassword == "" {
		return nil, errors.New("DynDns: ddns_password must be provided for the NoIP service")
	}

	ddns_service := config_obj.Frontend.DynDns.UpdateUrl
	if ddns_service == "" {
		ddns_service = "https://dynupdate.no-ip.com/"
	}

	if !strings.Contains(ddns_service, "{{.IP}}") {
		ddns_service = strings.TrimSuffix(ddns_service, "/") +
			"/nic/update?hostname={{.Hostname}}&myip={{.IP}}"
	}

	// Check the syntax of the template so we can exit early.
	template, err := template.New("url").Parse(ddns_service)
	if err != nil {
		return nil, fmt.Errorf(
			"DynDns: While parsing update template: %w", err)
	}

	ddns_userame := config_obj.Frontend.DynDns.DdnsUsername
	ddns_password := config_obj.Frontend.DynDns.DdnsPassword
	ddns_hostname := config_obj.Frontend.DynDns.DdnsHostname
	if ddns_hostname == "" {
		ddns_hostname = config_obj.Frontend.Hostname
	}

	return NoIPUpdater{
		username:     ddns_userame,
		password:     ddns_password,
		hostname:     ddns_hostname,
		service:      ddns_service,
		url_template: template,
	}, nil
}
