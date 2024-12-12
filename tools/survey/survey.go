package survey

import (
	"fmt"
	"regexp"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	url_validator = regexp.MustCompile("^[a-z0-9.A-Z\\-]+$")
	int_validator = regexp.MustCompile("^[0-9]+$")
)

func validate_int(message string) func(in string) error {
	return func(in string) error {
		if int_validator.MatchString(in) {
			return nil
		}
		return fmt.Errorf("%v: Invalid number: %v", message, in)
	}
}

type UserRecord struct {
	Name     string `json:"Name,omitempty"`
	Password string `json:"Password,omitempty"`
}

type ConfigSurvey struct {
	DatastoreLocation    string `json:"DatastoreLocation,omitempty"`
	LoggingPath          string `json:"LoggingPath,omitempty"`
	CertExpiration       string `json:"CertExpiration,omitempty"`
	ServerType           string `json:"ServerType,omitempty"`
	DeploymentType       string `json:"DeploymentType,omitempty"`
	Hostname             string `json:"Hostname,omitempty"`
	FrontendBindPort     string `json:"FrontendBindPort,omitempty"`
	GUIBindPort          string `json:"GUIBindPort,omitempty"`
	UseWebsocket         bool   `json:"UseWebsocket,omitempty"`
	SSOType              string `json:"SSOType,omitempty"`
	AzureTenantID        string `json:"AzureTenantID,omitempty"`
	OauthClientId        string `json:"OauthClientId,omitempty"`
	OauthClientSecret    string `json:"OauthClientSecret,omitempty"`
	OIDCIssuer           string `json:"OIDCIssuer,omitempty"`
	DynDNSType           string `json:"DynDNSType,omitempty"`
	ImplementAllowList   bool   `json:"ImplementAllowList,omitempty"`
	UseRegistryWriteback bool   `json:"UseRegistryWriteback,omitempty"`

	DdnsUsername string `json:"DdnsUsername,omitempty"`
	DdnsPassword string `json:"DdnsPassword,omitempty"`
	ZoneName     string `json:"ZoneName,omitempty"`
	ApiToken     string `json:"ApiToken,omitempty"`

	DefaultUsers []UserRecord `json:"DefaultUsers,omitempty"`

	// For frontend configurations
	MinionHostname string `json:"MinionHostname,omitempty"`
	MinionBindPort string `json:"MinionBindPort,omitempty"`
}

func GetInteractiveConfig() (*config_proto.Config, error) {

	// Build an intermediate survey config by asking the user
	// questions.
	config := &ConfigSurvey{
		Hostname:         "localhost",
		FrontendBindPort: "8000",
		GUIBindPort:      "8889",
	}

	err := getServerConfig(config)
	if err != nil {
		return nil, err
	}

	err = getNetworkConfig(config)
	if err != nil {
		return nil, err
	}

	err = configAuth(config)
	if err != nil {
		return nil, err
	}
	return config.Compile()
}

func required(name string) func(in string) error {
	return func(in string) error {
		if len(in) == 0 {
			return fmt.Errorf("%v must be set", name)
		}
		return nil
	}
}
