package ddclient

/*
This provider supports cloudflare for managing dynamic DNS

1. Sign into Cloudflare and buy a domain name.
2. go to https://dash.cloudflare.com/profile/api-tokens to generate an
   API token. Select "Edit Zone DNS" in the API Token templates.

You will require the "Edit" permission on Zone DNS and include the
specific zone name you want to manage. The zone name is the domain you
purchased for example "example.com". You will be able to set the
hostname under that domain, e.g. "velociraptor.example.com"

Using this information you can now create the dyndns configuration:

```yaml
Frontend:
  ....
  dyn_dns:
    type: cloudflare
    api_token: XXXYYYZZZ
    zone_name: example.com
```

Make sure the Frontend.Hostname field is set to the correct hostname
to update - for example

```yaml
Frontend:
  hostname: velociraptor.example.com
```

This is the hostname that will be updated.
*/

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type Zone struct {
	Name string `json:"name"`
	Id   string `json:"id"`
}

type ResponseError struct {
	Code       int             `json:"code"`
	Message    string          `json:"message"`
	ErrorChain []ResponseError `json:"error_chain"`
}

type ZonesResponse struct {
	Result  []Zone `json:"result"`
	Success bool   `json:"success"`
	Errors  []ResponseError
}

type ZonesResponseSingle struct {
	Result  Zone `json:"result"`
	Success bool `json:"success"`
	Errors  []ResponseError
}

type CloudflareUpdater struct {
	hostname, zone_name, token string
	client                     *http.Client
	config_obj                 *config_proto.Config
}

func (self CloudflareUpdater) setIPAddress(
	zone_id, record_id, external_ip string) error {

	_, err := self.getRequest("PATCH", fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%v/dns_records/%v",
		zone_id, record_id),
		json.Format(`{"content":%q, "ttl":60}`, external_ip))
	if err != nil {
		return err
	}

	return nil
}

func (self CloudflareUpdater) getRequest(
	method, url, body string) (*ZonesResponse, error) {

	var body_reader io.Reader
	if body != "" {
		body_reader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, body_reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", constants.USER_AGENT)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+self.token)

	resp, err := self.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res_body, err := utils.ReadAllWithLimit(resp.Body, constants.MAX_MEMORY)
	if err != nil {
		return nil, err
	}

	// Now parse these as json.
	result := &ZonesResponse{}
	err = json.Unmarshal(res_body, result)
	if err != nil {
		// Try a single response
		single := &ZonesResponseSingle{}
		err = json.Unmarshal(res_body, single)
		if err != nil {
			return nil, err
		}

		result.Result = append(result.Result, single.Result)
		result.Success = single.Success
		result.Errors = single.Errors
	}

	if !result.Success && len(result.Errors) > 0 {
		return result, errors.New(result.Errors[0].Message)
	}

	return result, nil
}

func (self CloudflareUpdater) getRecordId(zone_id string) (string, error) {

	result, err := self.getRequest("GET", fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%v/dns_records?name=%v&type=A",
		zone_id, self.hostname), "")
	if err != nil {
		return "", err
	}

	if len(result.Result) == 0 {
		return self.addRecordId(zone_id)
	}

	return result.Result[0].Id, nil
}

func (self CloudflareUpdater) addRecordId(zone_id string) (string, error) {
	result, err := self.getRequest("POST", fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%v/dns_records", zone_id),
		json.Format(`{"content":"127.0.0.1","name":%q,"type":"A","ttl":60}`,
			self.hostname))
	if err != nil {
		return "", err
	}

	if len(result.Result) == 0 {
		return "", errors.New("Record not added!")
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("DynDns: Added record %v for %v", result.Result[0].Id,
		result.Result[0].Name)

	return result.Result[0].Id, nil
}

func (self CloudflareUpdater) getZoneId() (string, error) {
	result, err := self.getRequest("GET",
		"https://api.cloudflare.com/client/v4/zones?name="+
			self.zone_name, "")
	if err != nil {
		return "", err
	}

	if len(result.Result) == 0 {
		return "", errors.New("Zone not found!")
	}

	return result.Result[0].Id, nil
}

func (self CloudflareUpdater) UpdateDDNSRecord(
	ctx context.Context, config_obj *config_proto.Config,
	external_ip string) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	zone_id, err := self.getZoneId()
	if err != nil {
		return err
	}

	logger.Debug("DynDns: Zone Id %v", zone_id)

	record_id, err := self.getRecordId(zone_id)
	if err != nil {
		return err
	}

	logger.Debug("DynDns: Record Id %v", record_id)

	err = self.setIPAddress(zone_id, record_id, external_ip)

	return err
}

func NewCloudflareUpdater(config_obj *config_proto.Config) (Updater, error) {
	if config_obj.Frontend.DynDns.ApiToken == "" {
		return nil, errors.New("DynDns: API token must be provided for the Cloudflare service")
	}

	if config_obj.Frontend.DynDns.ApiToken == "" {
		return nil, errors.New("DynDns: Zone name is required for the Cloudflare service")
	}

	return CloudflareUpdater{
		hostname:   config_obj.Frontend.Hostname,
		token:      config_obj.Frontend.DynDns.ApiToken,
		zone_name:  config_obj.Frontend.DynDns.ZoneName,
		config_obj: config_obj,
		client: &http.Client{
			CheckRedirect: nil,
			Transport: &http.Transport{
				Proxy: networking.GetProxy(),
			},
		},
	}, nil
}
