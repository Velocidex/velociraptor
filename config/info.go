package config

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

func GoogleAuthEnabled(config_obj *config_proto.Config) bool {
	return config_obj.GUI.GoogleOauthClientId != "" &&
		config_obj.GUI.GoogleOauthClientSecret != ""
}

func SAMLEnabled(config_obj *config_proto.Config) bool {
	return config_obj.GUI.SamlCertificate != "" && config_obj.GUI.SamlPrivateKey != "" &&
		config_obj.GUI.SamlIdpMetadataUrl != "" && config_obj.GUI.SamlRootUrl != ""
}
