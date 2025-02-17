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
  "verifier":"x=>(x.credentials_key AND x.credentials_secret) OR x.credentials_token OR x.serverside_encryption OR x.kms_encryption_key",
  "description": "Credentials used to interact with S3 buckets.",
  "template": {
     "region": "us-east-1",
     "skip_verify": "",
     "credentials_key": "",
     "credentials_secret": "",
     "credentials_token": "",
     "endpoint": "",
     "serverside_encryption": "",
     "kms_encryption_key": ""
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
}`, `{
  "typeName":"HTTP Secrets",
  "description": "Credentials to be used in HTTP requests with http_client() calls.",
  "template": {
     "url": "",
     "url_regex": "",
     "method": "",
     "user_agent": "",
     "root_ca": "",
     "skip_verify": "FALSE",
     "extra_params": "# Add extra parameters as YAML strings\n#Foo: Value\n#Baz:Value2\n",
     "extra_headers": "# Add extra headers as YAML strings\n#Authorization: Value\n",
     "cookies": "# Add cookies as YAML strings\n#Cookie1: Value\n#Cookie2: Value2\n"
  },
  "verifier": "x=>x.url || x.url_regex"
}`, `{
  "typeName":"Splunk Creds",
  "description": "Credentials to be used in upload_splunk() calls.",
  "template": {
     "url": "",
     "token": "",
     "index": "",
     "source": "",
     "root_ca": "",
     "hostname": "",
     "hostname_field": "",
     "skip_verify": "FALSE"
  },
  "verifier": "x=>x.index AND x.url"
}`, `{
  "typeName":"Elastic Creds",
  "description": "Credentials to be used in upload_elastic() calls.",
  "template": {
     "index": "",
     "type": "",
     "addresses": "# Add URLs one per line\n# http://www.example.com/\n",
     "username": "",
     "password": "",
     "cloud_id": "",
     "api_key": "",
     "pipeline": "",
     "root_ca": "",
     "action": "",
     "skip_verify": "FALSE"
  },
  "verifier": "x=>x.addresses"
}`, `{
  "typeName":"SMTP Creds",
  "description": "Credentials to be used in mail() plugin.",
  "template": {
     "server": "127.0.0.1",
     "server_port": "587",
     "auth_username": "",
     "auth_password": "",
     "skip_verify": "FALSE"
  },
  "verifier": "x=>x.server && x.server_port"
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
