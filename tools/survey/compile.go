package survey

import (
	"encoding/hex"
	"fmt"
	"path"

	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Build a config file from the intermediate survey
func (self *ConfigSurvey) Compile() (*config_proto.Config, error) {
	config_obj := config.GetDefaultConfig()

	if config_obj.Security == nil {
		config_obj.Security = &config_proto.Security{}
	}

	cert_expiry, _ := utils.ToInt64(self.CertExpiration)
	if cert_expiry == 0 {
		cert_expiry = 1
	}
	config_obj.Security.CertificateValidityDays = 365 * cert_expiry

	if self.UseRegistryWriteback {
		config_obj.Client.WritebackWindows = "HKLM\\SOFTWARE\\Velocidex\\Velociraptor"
	}

	// Generate new keys
	err := GenerateNewKeys(config_obj)
	if err != nil {
		return nil, err
	}

	config_obj.Datastore.Implementation = "FileBaseDataStore"
	if self.DatastoreLocation == "" {
		self.DatastoreLocation = self.DefaultDatastoreLocation()
	}
	config_obj.Datastore.Location = self.DatastoreLocation
	config_obj.Datastore.FilestoreDirectory = self.DatastoreLocation

	config_obj.Logging.SeparateLogsPerComponent = true

	if self.LoggingPath == "" {
		self.LoggingPath = path.Join(self.DatastoreLocation, "logs")
	}
	config_obj.Logging.OutputDirectory = self.LoggingPath

	// By default disabled debug logging - it is not useful unless
	// you are trying to debug something.
	config_obj.Logging.Debug = &config_proto.LoggingRetentionConfig{
		Disabled: true,
	}

	config_obj.Frontend.Hostname = self.Hostname

	port, _ := utils.ToInt64(self.FrontendBindPort)
	config_obj.Frontend.BindPort = uint32(port)

	port, _ = utils.ToInt64(self.GUIBindPort)
	config_obj.GUI.BindPort = uint32(port)

	config_obj.GUI.PublicUrl = fmt.Sprintf(
		"https://%s:%d/app/index.html", config_obj.Frontend.Hostname,
		config_obj.GUI.BindPort)

	config_obj.GUI.Authenticator.Type = "Basic"

	switch self.DeploymentType {
	case "self_signed":
		config_obj.GUI.BindAddress = "127.0.0.1"
		config_obj.Frontend.BindAddress = "0.0.0.0"
		config_obj.Client.UseSelfSignedSsl = true
		config_obj.Client.ServerUrls = []string{
			self.getURL(config_obj.Frontend)}

	case "oauth_sso":
		config_obj.GUI.Authenticator.Type = self.SSOType
		config_obj.GUI.Authenticator.OauthClientId = self.OauthClientId
		config_obj.GUI.Authenticator.OauthClientSecret = self.OauthClientSecret
		config_obj.GUI.Authenticator.Tenant = self.AzureTenantID
		config_obj.GUI.Authenticator.OidcIssuer = self.OIDCIssuer

		fallthrough

	case "autocert":
		config_obj.Frontend.BindPort = 443
		config_obj.Frontend.BindAddress = "0.0.0.0"
		config_obj.GUI.BindPort = 443
		config_obj.GUI.BindAddress = "0.0.0.0"
		config_obj.GUI.PublicUrl = fmt.Sprintf(
			"https://%s/app/index.html", config_obj.Frontend.Hostname)

		config_obj.Client.ServerUrls = []string{
			self.getURL(config_obj.Frontend)}

		config_obj.AutocertCertCache = config_obj.Datastore.Location

	}

	switch self.DynDNSType {
	case "noip":
		config_obj.Frontend.DynDns = &config_proto.DynDNSConfig{
			Type:         "noip",
			DdnsUsername: self.DdnsUsername,
			DdnsPassword: self.DdnsPassword,
		}
	case "cloudflare":
		config_obj.Frontend.DynDns = &config_proto.DynDNSConfig{
			Type:     "cloudflare",
			ZoneName: self.ZoneName,
			ApiToken: self.ApiToken,
		}
	default:
		config_obj.Frontend.DynDns = nil
	}

	// Now add users
	for _, user := range self.DefaultUsers {
		user_record, err := users.NewUserRecord(config_obj, user.Name)
		if err != nil {
			continue
		}

		if config_obj.GUI.Authenticator.Type != "sso_type" {
			users.SetPassword(user_record, user.Password)
		}

		config_obj.GUI.InitialUsers = append(
			config_obj.GUI.InitialUsers,
			&config_proto.GUIUser{
				Name:         user_record.Name,
				PasswordHash: hex.EncodeToString(user_record.PasswordHash),
				PasswordSalt: hex.EncodeToString(user_record.PasswordSalt),
			})
	}

	if self.ImplementAllowList {
		config_obj.Security.AllowedPlugins = allowed_plugins
		config_obj.Security.AllowedFunctions = allowed_functions
		config_obj.Security.AllowedAccessors = allowed_accessors
	}

	return config_obj, nil
}

func (self *ConfigSurvey) DefaultDatastoreLocation() string {
	switch self.ServerType {
	case "windows":
		return "C:\\Windows\\Temp"
	case "linux":
		return "/opt/velociraptor"
	default:
		return self.DatastoreLocation
	}
}
