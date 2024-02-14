package sanity

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

var built_in_definitions = []string{`{
  "typeName":"AWS S3 Creds",
  "verifier":"x=>(x.credentials_key AND x.credentials_secret) OR x.credentials_token OR x.serversideencryption OR x.kmsencryptionkey",
  "description": "Credentials used to interact with S3 buckets. Not all fields should be filled and only certain combinations are valid.",
  "template": {
     "region": "us-east-1",
     "credentials_key": "",
     "credentials_secret": "",
     "credentials_token": "",
     "endpoint": "",
     "serversideencryption": "",
     "kmsencryptionkey": ""
  }
}`, `{
  "typeName":"SSH PrivateKey",
  "description": "SSH Credentials in the form of a private_key and public key",
  "template": {
     "username": "",
     "password": "",
     "private_key": "",
     "hostname": ""
  },
  "verifier": "x=>x.username AND x.hostname =~ ':[0-9]+$' AND (x.password OR x.private_key =~ 'BEGIN OPENSSH PRIVATE KEY')"
}`,
}

func (self *SanityChecks) createBuiltInSecretDefinitions(
	ctx context.Context, config_obj *config_proto.Config) error {

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return err
	}

	for _, def := range built_in_definitions {
		definition := &api_proto.SecretDefinition{}
		err := protojson.Unmarshal([]byte(def), definition)
		if err != nil {
			return err
		}
		definition.BuiltIn = true

		err = secrets_service.DefineSecret(ctx, definition)
		if err != nil {
			return err
		}
	}

	return nil
}
