package s3

import (
	"crypto/tls"
	"net/http"

	"github.com/Velocidex/ordereddict"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
)

const (
	S3_CREDENTIALS = "S3_CREDENTIALS"
	S3_TAG         = "_S3_TAG"
)

func GetS3Session(scope vfilter.Scope) (*session.Session, error) {
	// Empty credentials are OK - they just mean to get creds from the
	// process env
	setting, pres := scope.Resolve(S3_CREDENTIALS)
	if !pres {
		setting = ordereddict.NewDict()
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
