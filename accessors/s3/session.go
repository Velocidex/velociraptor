package s3

import (
	"context"
	"errors"
	"net/http"

	"github.com/Velocidex/ordereddict"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	vconfig "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
)

const (
	S3_TAG = "_S3_TAG"
)

func GetS3Client(
	ctx context.Context,
	scope vfilter.Scope) (res *s3.Client, err error) {

	// Empty credentials are OK - they just mean to get creds from the
	// process env
	setting, pres := scope.Resolve(constants.S3_CREDENTIALS)
	if !pres {
		setting = ordereddict.NewDict()
	}

	// Check for a secret from the secrets service
	secret := vql_subsystem.GetStringFromRow(scope, setting, "secret")
	if secret != "" {
		setting, err = getSecret(ctx, scope, secret)
		if err != nil {
			return nil, err
		}
	}

	conf := []func(*config.LoadOptions) error{}
	region := vql_subsystem.GetStringFromRow(scope, setting, "region")
	if region != "" {
		conf = append(conf, config.WithRegion(region))
	}

	credentials_key := vql_subsystem.GetStringFromRow(
		scope, setting, "credentials_key")

	credentials_secret := vql_subsystem.GetStringFromRow(
		scope, setting, "credentials_secret")

	token := vql_subsystem.GetStringFromRow(
		scope, setting, "credentials_token")

	if credentials_key != "" && credentials_secret != "" {
		conf = append(conf, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				credentials_key, credentials_secret, token),
		))
	}

	s3_opts := []func(*s3.Options){}

	endpoint := vql_subsystem.GetStringFromRow(
		scope, setting, "endpoint")

	if endpoint != "" {
		s3_opts = append(s3_opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})

		cert_no_verify := vql_subsystem.GetBoolFromRow(
			scope, setting, "skip_verify")

		if cert_no_verify {
			config_obj, pres := vql_subsystem.GetServerConfig(scope)
			if !pres {
				config_obj = vconfig.GetDefaultConfig()
			}

			tlsConfig, err := networking.GetSkipVerifyTlsConfig(
				config_obj.Client)
			if err != nil {
				return nil, err
			}

			tr := &http.Transport{
				Proxy:           networking.GetProxy(),
				TLSClientConfig: tlsConfig,
			}

			http_client := &http.Client{Transport: tr}
			conf = append(conf, config.WithHTTPClient(http_client))
		}
	}

	sess, err := config.LoadDefaultConfig(ctx, conf...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(sess, s3_opts...)

	return client, nil
}

func getSecret(
	ctx context.Context,
	scope vfilter.Scope, secret string) (
	*ordereddict.Dict, error) {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil, errors.New("Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return nil, err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	secret_record, err := secrets_service.GetSecret(ctx, principal,
		constants.AWS_S3_CREDS, secret)
	if err != nil {
		return nil, err
	}

	return secret_record.Data, nil
}
