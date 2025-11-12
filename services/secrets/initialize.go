package secrets

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
)

var (
	/* Although it may seem that we can support arbitrary secret
	   definitions, this is not the case. Secrects can only be used by
	   VQL plugins and functions that expects them and are never
	   exposed to VQL queries. Therefore defining custom secrets is
	   useless.
	*/
	built_in_definitions = []*api_proto.SecretDefinition{{
		TypeName:    constants.AWS_S3_CREDS,
		Description: "Credentials used to interact with S3 buckets.",
		Verifier: `x=>(x.credentials_key AND x.credentials_secret)
                       OR x.credentials_token
                       OR x.serverside_encryption
                       OR x.kms_encryption_key`,
		Fields: []string{
			"region",
			"skip_verify",
			"credentials_key",
			"credentials_secret",
			"credentials_token",
			"endpoint",
			"serverside_encryption",
			"kms_encryption_key",
			"path_style",
		},
		Template: map[string]string{
			"region": "us-east-1",
		},
	}, {
		TypeName:    constants.SSH_PRIVATE_KEY,
		Description: "SSH Credentials in the form of a private_key and public key",
		Verifier:    "x=>x.username AND x.hostname AND ( x.password OR x.private_key) ",
		Fields: []string{
			"username",
			"password",
			"private_key",
			"hostname",
		},
		Template: map[string]string{
			"username": "uploader",
		},
	}, {
		TypeName:    constants.HTTP_SECRETS,
		Description: "Credentials to be used in HTTP requests with http_client() calls.",

		// Empty method means GET. If a method is defined it must be one of these.
		Verifier: `x=>( x.url OR x.url_regex ) AND x.method =~ "GET|POST|PUT|DELETE|^$"`,
		Fields: []string{
			"url",
			"url_regex",
			"method",
			"user_agent",
			"root_ca",
			"skip_verify",
			"extra_params",
			"extra_headers",
			"cookies",
		},
		Template: map[string]string{
			"skip_verify":   "FALSE",
			"extra_params":  "# Add extra parameters as YAML strings\n#Foo: Value\n#Baz:Value2\n",
			"extra_headers": "# Add extra headers as YAML strings\n#Authorization: Value\n",
			"cookies":       "# Add cookies as YAML strings\n#Cookie1: Value\n#Cookie2: Value2\n",
		},
	}, {
		TypeName:    constants.SPLUNK_CREDS,
		Description: "Credentials to be used in upload_splunk() calls.",
		Verifier:    `x=>x.index AND x.url`,
		Fields: []string{
			"url",
			"token",
			"index",
			"source",
			"root_ca",
			"hostname",
			"hostname_field",
			"skip_verify",
		},
		Template: map[string]string{
			"skip_verify": "FALSE",
		},
	}, {
		TypeName:    constants.ELASTIC_CREDS,
		Description: "Credentials to be used in upload_elastic() calls.",
		Verifier:    "x=>x.addresses",
		Fields: []string{
			"index",
			"type",
			"addresses",
			"username",
			"password",
			"cloud_id",
			"api_key",
			"pipeline",
			"root_ca",
			"action",
			"skip_verify",
		},
		Template: map[string]string{
			"addresses":   "# Add URLs one per line\n# http://www.example.com/\n",
			"skip_verify": "FALSE",
		},
	}, {
		TypeName:    constants.SMTP_CREDS,
		Verifier:    "x=>x.server && x.server_port",
		Description: "Credentials to be used in mail() plugin.",
		Fields: []string{
			"server",
			"server_port",
			"auth_username",
			"auth_password",
			"skip_verify",
			"from",
			"root_ca",
		},
		Template: map[string]string{
			"server":      "127.0.0.1",
			"server_port": "587",
			"skip_verify": "FALSE",
		},
	}, {
		TypeName:    constants.EXECVE_SECRET,
		Description: "Enforce a prefix command on the execve() plugin",
		Verifier:    "x=>x.prefix_commandline",
		Fields: []string{
			"prefix_commandline",
			"env",
			"cwd",
		},
		Template: map[string]string{
			"env": "# Add extra parameters as YAML strings\n#Foo: Value\n#Baz:Value2\n",
		},
	}}
)

func buildInitialSecretDefinitions() map[string]*SecretDefinition {
	res := make(map[string]*SecretDefinition)

	for _, definition := range built_in_definitions {
		def, err := NewSecretDefinition(definition)
		if err != nil {
			// This can not happen since the definitions are hard
			// coded.
			panic(definition)
		}
		res[definition.TypeName] = def
	}

	return res
}
