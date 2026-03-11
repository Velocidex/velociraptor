//go:build !sumo
// +build !sumo

package tools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/encrypt"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type S3UploadArgs struct {
	File                 *accessors.OSPath `vfilter:"required,field=file,doc=The file to upload"`
	Name                 string            `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server"`
	Accessor             string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Bucket               string            `vfilter:"required,field=bucket,doc=The bucket to upload to"`
	Region               string            `vfilter:"optional,field=region,doc=The region the bucket is in"`
	CredentialsKey       string            `vfilter:"optional,field=credentials_key,doc=The AWS key credentials to use"`
	CredentialsSecret    string            `vfilter:"optional,field=credentials_secret,doc=The AWS secret credentials to use"`
	CredentialsToken     string            `vfilter:"optional,field=credentials_token,doc=The AWS session token to use (only needed for temporary credentials)"`
	Endpoint             string            `vfilter:"optional,field=endpoint,doc=The Endpoint to use"`
	ServerSideEncryption string            `vfilter:"optional,field=serverside_encryption,doc=The server side encryption method to use"`
	KmsEncryptionKey     string            `vfilter:"optional,field=kms_encryption_key,doc=The server side KMS key to use"`
	S3UploadRoot         string            `vfilter:"optional,field=s3upload_root,doc=Prefix for the S3 object"`
	SkipVerify           bool              `vfilter:"optional,field=skip_verify,doc=Skip TLS Verification"`
	UsePathStyle         bool              `vfilter:"optional,field=path_style,doc=Use path style URLs if set"`
	Secret               string            `vfilter:"optional,field=secret,doc=Alternatively use a secret from the secrets service. Secret must be of type 'AWS S3 Creds'"`
}

type S3UploadFunction struct{}

func (self S3UploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "upload_s3", args)()

	scope.Log("DEBUG:upload_s3 is built with the minio library. For full features rebuild with the `sumo` option")

	mergeScope(ctx, scope, args)

	arg := &S3UploadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload_S3: %s", err.Error())
		return vfilter.Null{}
	}

	err = self.maybeForceSecrets(ctx, scope, arg)
	if err != nil {
		scope.Log("upload_S3: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Secret != "" {
		err := mergeSecret(ctx, scope, arg)
		if err != nil {
			scope.Log("upload_S3: %s", err)
			return vfilter.Null{}
		}
	}

	err = vql_subsystem.CheckAccess(scope, acls.NETWORK)
	if err != nil {
		scope.Log("upload_S3: %s", err)
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_S3: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_S3: Unable to open %s: %s",
			arg.File, err.Error())
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File.String()
	}

	stat, err := accessor.LstatWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_S3: Unable to stat %s: %v",
			arg.File, err)
	} else if !stat.IsDir() {
		// Abort uploading when the scope is destroyed.
		sub_ctx, cancel := context.WithCancel(ctx)
		// Cancel the s3 upload when the scope destroys.
		_ = scope.AddDestructor(cancel)
		upload_response, err := upload_S3(
			sub_ctx, scope, file, arg, uint64(stat.Size()))
		if err != nil {
			scope.Log("upload_S3: %v", err)

			// Relay the error in the UploadResponse
			return upload_response
		}
		return upload_response
	}

	return vfilter.Null{}
}

func GetS3Client(
	ctx context.Context, scope vfilter.Scope,
	arg *S3UploadArgs) (*minio.Client, error) {
	s3_opts := &minio.Options{}
	if arg.CredentialsKey != "" && arg.CredentialsSecret != "" {
		s3_opts.Creds = credentials.NewStaticV4(
			arg.CredentialsKey, arg.CredentialsSecret, arg.CredentialsToken)
	}

	endpoint := arg.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}

	if strings.HasPrefix(endpoint, "http") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("Unable to parse endpoint URL %v", endpoint)
		}

		s3_opts.Secure = u.Scheme == "https"
		endpoint = u.Host
	}

	clientConfig, ok := artifacts.GetConfig(scope)
	if ok {
		if arg.SkipVerify {
			http_client, err := networking.GetSkipVerifyHTTPClient(
				ctx, clientConfig, scope, "", nil)
			if err != nil {
				return nil, err
			}
			s3_opts.Transport = http_client.Transport()
		} else {
			http_client, err := networking.GetDefaultHTTPClient(
				ctx, clientConfig, scope, "", nil)
			if err != nil {
				return nil, err
			}
			s3_opts.Transport = http_client.Transport()
		}
	}

	return minio.New(endpoint, s3_opts)
}

func upload_S3(ctx context.Context, scope vfilter.Scope,
	reader accessors.ReadSeekCloser,
	arg *S3UploadArgs,
	size uint64) (
	*uploads.UploadResponse, error) {

	if arg.S3UploadRoot != "" {
		arg.Name = arg.S3UploadRoot + arg.Name
	}
	scope.Log("upload_S3: Uploading %v to %v", arg.Name, arg.Bucket)

	s3Client, err := GetS3Client(ctx, scope, arg)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	put_options := minio.PutObjectOptions{}
	if arg.ServerSideEncryption != "" {
		// Create a key based on the password and bucket and object names.
		encryption := encrypt.DefaultPBKDF(
			[]byte(arg.ServerSideEncryption), []byte(arg.Bucket+arg.Name))
		put_options.ServerSideEncryption = encryption
	}

	if arg.KmsEncryptionKey != "" {
		err := fmt.Errorf("KmsEncryptionKey not supported, rebuild with sumo option")
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	upload_info, err := s3Client.PutObject(
		ctx, arg.Bucket, arg.Name, reader, int64(size), put_options)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	// All good! report the outcome.
	response := &uploads.UploadResponse{
		Path: upload_info.Location,
	}

	response.Size = uint64(upload_info.Size)
	return response, nil
}

func (self S3UploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "upload_s3",
		Doc:     "Upload files to S3.",
		ArgType: type_map.AddType(scope, &S3UploadArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.NETWORK, acls.FILESYSTEM_READ).Build(),
		Version: 3,
	}
}

func (self S3UploadFunction) maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope, arg *S3UploadArgs) error {

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

var critical_fields = []string{
	"secret", "region",
	"credentials_key", "credentials_secret", "credentials_token",
	"endpoint", "skip_verify",
	"serverside_encryption", "kms_encryption_key",
}

// The S3 accessor can be configured with the S3_CREDENTIALS scope
// variable. This function looks to that variable for configuration.
func mergeScope(ctx context.Context,
	scope vfilter.Scope, args *ordereddict.Dict) {

	for _, key := range critical_fields {
		_, pres := args.Get(key)
		if pres {
			// Do not read settings from scope.
			return
		}
	}

	// Are there settings in scope?
	setting, pres := scope.Resolve(constants.S3_CREDENTIALS)
	if !pres {
		// Nope.
		return
	}

	for _, key := range critical_fields {
		value, pres := scope.Associative(setting, key)
		if !pres {
			continue
		}
		// These values are boolean actually.
		if key == "skip_verify" {
			value = scope.Bool(value)
		}

		args.Set(key, value)
	}
}

func mergeSecret(ctx context.Context, scope vfilter.Scope, arg *S3UploadArgs) error {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return errors.New("Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	s, err := secrets_service.GetSecret(ctx, principal,
		constants.AWS_S3_CREDS, arg.Secret)
	if err != nil {
		return err
	}

	arg.Region = s.GetString("region")
	arg.CredentialsKey = s.GetString("credentials_key")
	arg.CredentialsSecret = s.GetString("credentials_secret")
	arg.CredentialsToken = s.GetString("credentials_token")
	arg.Endpoint = s.GetString("endpoint")
	arg.ServerSideEncryption = s.GetString("serverside_encryption")
	arg.KmsEncryptionKey = s.GetString("kms_encryption_key")
	arg.UsePathStyle = s.GetBool("path_style")

	return nil
}

func init() {
	vql_subsystem.RegisterFunction(&S3UploadFunction{})
}
