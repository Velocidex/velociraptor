package s3

import (
	"context"
	"errors"

	"github.com/Velocidex/ordereddict"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/utils/dict"
)

const (
	S3_TAG = "_S3_TAG"
)

type S3AcccessorArgs struct {
	Secret            string `vfilter:"optional,field=secret,doc=The name of a secret to use."`
	Region            string `vfilter:"optional,field=region,doc=The region."`
	CredentialsKey    string `vfilter:"optional,field=credentials_key"`
	CredentialsSecret string `vfilter:"optional,field=credentials_secret"`
	CredentialsToken  string `vfilter:"optional,field=credentials_token"`
	Endpoint          string `vfilter:"optional,field=endpoint"`
	SkipVerify        bool   `vfilter:"optional,field=skip_verify"`
}

func GetS3Client(
	ctx context.Context,
	scope vfilter.Scope) (res *s3.Client, err error) {

	// Empty credentials are OK - they just mean to get creds from the
	// process env
	setting, pres := scope.Resolve(constants.S3_CREDENTIALS)
	if !pres {
		setting = ordereddict.NewDict()
	}

	args := dict.RowToDict(ctx, scope, setting)
	arg := &S3AcccessorArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		return nil, err
	}

	err = maybeForceSecrets(ctx, scope, arg)
	if err != nil {
		return nil, err
	}

	// Check for a secret from the secrets service
	if arg.Secret != "" {
		arg, err = getSecret(ctx, scope, arg.Secret)
		if err != nil {
			return nil, err
		}
	}

	conf := []func(*config.LoadOptions) error{}
	if arg.Region != "" {
		conf = append(conf, config.WithRegion(arg.Region))
	}

	if arg.CredentialsKey != "" && arg.CredentialsSecret != "" {
		conf = append(conf, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				arg.CredentialsKey, arg.CredentialsSecret,
				arg.CredentialsToken),
		))
	}

	s3_opts := []func(*s3.Options){}
	if arg.Endpoint != "" {
		s3_opts = append(s3_opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(arg.Endpoint)
		})
	}

	clientConfig, ok := artifacts.GetConfig(scope)
	if ok {
		if arg.SkipVerify {
			http_client, err := networking.GetSkipVerifyHTTPClient(
				ctx, clientConfig, scope, "", nil)
			if err != nil {
				return nil, err
			}

			conf = append(conf, config.WithHTTPClient(http_client))

		} else {
			http_client, err := networking.GetDefaultHTTPClient(
				ctx, clientConfig, scope, "", nil)
			if err != nil {
				return nil, err
			}
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

func maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope, arg *S3AcccessorArgs) error {

	// Not running on the server, secrets dont work.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil
	}

	if config_obj.Security == nil {
		return nil
	}

	if !config_obj.Security.VqlMustUseSecrets {
		return nil
	}

	// If an explicit secret is defined let it filter the URLs.
	if arg.Secret != "" {
		return nil
	}

	return utils.SecretsEnforced
}

func getSecret(
	ctx context.Context,
	scope vfilter.Scope, secret string) (*S3AcccessorArgs, error) {
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

	arg := &S3AcccessorArgs{
		Region:            secret_record.GetString("region"),
		CredentialsKey:    secret_record.GetString("credentials_key"),
		CredentialsSecret: secret_record.GetString("credentials_secret"),
		CredentialsToken:  secret_record.GetString("credentials_token"),
		Endpoint:          secret_record.GetString("endpoint"),
		SkipVerify:        secret_record.GetBool("skip_verify"),
	}
	return arg, nil
}
