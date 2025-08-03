package services

/*
  This service manages secrets to be used inside the VQL environment.

  Many VQL plugins and functions require a secret to perform their
  role. For example, the SSH accessor requires ssh credentials to log
  into the remote system. The upload_s3() function requires S3
  credentials to upload files to the cloud.

  Traditionally credentials were provided directly to these VQL
  functions via VQL parameters or VQL environment variables. This
  works and it is the most direct way but it makes it difficult to
  protect these secrets from malicious use and to prevent them from
  leaking unintentionally.

  For example, consider an artifact that uploads data to Elastic. That
  artifact needs API credentials to push to elastic and these need to
  be provided in the GUI as an artifact parameter. This means that:

  1. The user managing the server needs to now have access to Elastic credentials.

  2. If we are not careful, anyone viewing the artifact in the GUI can
     just read the credentials as parameters (It is possible to set
     parameter types to redacted to ensure the GUI redacts these
     parameters).

  ## Dedicated secret managements

  The secrets service solves this issue by managing secrets outside of
  VQL. Once a secret is registered with the secrets service by name,
  the user is unable to retrieve the secret from VQL
  directly. Instead, the VQL plugins that require that secret can ask
  for it in code providing the identity of the principal under which
  the query is run.

  If the principal is allowed to use the secret the service will
  return the service for use by the plugin.

  Now there is no risk of any secrets leaking in the VQL
  environment. For example, the following query:

  SELECT upload_s3(secret="MyS3Credentials", path=OSPath) FROM scope()

  Will retrieve the `MyS3Credentials` if the calling user is allowed
  and the file will be uploaded to s3, but there is no possibility any
  more of retrieving the plain text secret from VQL.

  Equally, if another user copies the artifact, they can not
  automatically use the secret, unless they too have access to it.

  This allows more careful management of secrets and reduces
  opportunity for credential leaks.

*/

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type Secret struct {
	*api_proto.Secret

	Data *ordereddict.Dict
}

type SecretsService interface {
	GetSecretDefinitions(ctx context.Context) []*api_proto.SecretDefinition

	// Add a new managed secret. This function applies any verifiers
	// to ensure the secret is valid.
	AddSecret(ctx context.Context, scope vfilter.Scope,
		type_name, secret_name string,
		secret *ordereddict.Dict) error

	// Grants access to the secret to the specified users.
	ModifySecret(ctx context.Context,
		request *api_proto.ModifySecretRequest) error

	// Retrieve a secret. This function may only be called internally
	// from VQL plugins/functions. The secrets may not be leaked into
	// the VQL environment. The function checks the principal against
	// the secret's ACLs to ensure they are allowed access to it.
	GetSecret(ctx context.Context,
		principal, type_name, secret_name string) (*Secret, error)

	GetSecretMetadata(ctx context.Context,
		type_name, secret_name string) (*Secret, error)
}

func GetSecretsService(config_obj *config_proto.Config) (SecretsService, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}
	return org_manager.Services(config_obj.OrgId).SecretsService()
}

// Utilities to extract secrets
func (self *Secret) GetString(field string, target *string) {
	res, pres := self.Data.GetString(field)
	if pres && res != "" {
		*target = res
	}
}

func (self *Secret) GetStrings(field string, target *[]string) {
	res, pres := self.Data.GetString(field)
	if pres && res != "" {
		*target = []string{}
		for _, line := range strings.Split(res, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				continue
			}
			*target = append(*target, line)
		}
	}
}

func (self *Secret) GetBool(field string, target *bool) {
	res, pres := self.Data.GetString(field)
	if pres && res != "" {
		*target = vql_subsystem.GetBoolFromString(res)
	}
}

func (self *Secret) GetUint64(field string, target *uint64) {
	res, pres := self.Data.GetString(field)
	if pres && res != "" {
		res_int, _ := strconv.ParseInt(res, 0, 64)
		*target = uint64(res_int)
	}
}

// We expect Dict parameters to be a YAML formatted object.
func (self *Secret) GetDict(field string, target *ordereddict.Dict) error {
	res, pres := self.Data.GetString(field)
	if pres && res != "" {
		tmp := make(map[string]string)
		err := yaml.Unmarshal([]byte(res), tmp)
		if err != nil {
			return fmt.Errorf("Secret: parsing field %v invalid yaml: %v",
				field, err)
		}
		for k, v := range tmp {
			if v != "" {
				target.Set(k, v)
			}
		}
	}

	return nil
}
