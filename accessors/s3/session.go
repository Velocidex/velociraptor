package s3

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"

	"github.com/Velocidex/ordereddict"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
)

const (
	S3_TAG = "_S3_TAG"
)

func GetS3Session(scope vfilter.Scope) (res *session.Session, err error) {
	// Empty credentials are OK - they just mean to get creds from the
	// process env
	setting, pres := scope.Resolve(constants.S3_CREDENTIALS)
	if !pres {
		setting = ordereddict.NewDict()
	}

	// Check for a secret from the secrets service
	secret := vql_subsystem.GetStringFromRow(scope, setting, "secret")
	if secret != "" {
		setting, err = getSecret(scope, secret)
		if err != nil {
			return nil, err
		}
	}

	conf := aws.NewConfig()
	region := vql_subsystem.GetStringFromRow(scope, setting, "region")
	if region != "" {
		conf = conf.WithRegion(region)
	}

	credentials_key := vql_subsystem.GetStringFromRow(
		scope, setting, "credentials_key")

	credentials_secret := vql_subsystem.GetStringFromRow(
		scope, setting, "credentials_secret")

	if credentials_key != "" && credentials_secret != "" {
		token := ""
		creds := credentials.NewStaticCredentials(
			credentials_key, credentials_secret, token)
		_, err := creds.Get()
		if err != nil {
			return nil, err
		}

		conf = conf.WithCredentials(creds)
	}

	endpoint := vql_subsystem.GetStringFromRow(
		scope, setting, "endpoint")

	if endpoint != "" {
		conf = conf.WithEndpoint(endpoint).
			WithS3ForcePathStyle(true)

		cert_no_verify := vql_subsystem.GetBoolFromRow(
			scope, setting, "cert_no_verify")

		if cert_no_verify {
			tr := &http.Transport{
				Proxy:           networking.GetProxy(),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}

			client := &http.Client{Transport: tr}
			conf = conf.WithHTTPClient(client)
		}
	}

	sess, err := session.NewSessionWithOptions(
		session.Options{
			Config:            *conf,
			SharedConfigState: session.SharedConfigEnable,
		})
	if err != nil {
		return nil, err
	}

	return sess, nil
}

func getSecret(scope vfilter.Scope, secret string) (
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

	// Extract the context from the scope.
	ctx := context.TODO()

	secret_record, err := secrets_service.GetSecret(ctx, principal,
		constants.AWS_S3_CREDS, secret)
	if err != nil {
		return nil, err
	}

	return secret_record.Data, nil
}
