//go:build extras

package tools

import (
	"crypto/tls"
	"net/http"

	"github.com/Velocidex/ordereddict"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/net/context"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/uploads"
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
	Region               string            `vfilter:"required,field=region,doc=The region the bucket is in"`
	CredentialsKey       string            `vfilter:"optional,field=credentialskey,doc=The AWS key credentials to use"`
	CredentialsSecret    string            `vfilter:"optional,field=credentialssecret,doc=The AWS secret credentials to use"`
	Endpoint             string            `vfilter:"optional,field=endpoint,doc=The Endpoint to use"`
	ServerSideEncryption string            `vfilter:"optional,field=serversideencryption,doc=The server side encryption method to use"`
	KmsEncryptionKey     string            `vfilter:"optional,field=kmsencryptionkey,doc=The server side KMS key to use"`
	S3UploadRoot         string            `vfilter:"optional,field=s3uploadroot,doc=Prefix for the S3 object"`
	NoVerifyCert         bool              `vfilter:"optional,field=noverifycert,doc=Skip TLS Verification (deprecated in favor of SkipVerify)"`
	SkipVerify           bool              `vfilter:"optional,field=skip_verify,doc=Skip TLS Verification"`
}

type S3UploadFunction struct{}

func (self *S3UploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &S3UploadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload_S3: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.NoVerifyCert {
		scope.Log("upload_S3: NoVerifyCert is deprecated, please use SkipVerify")
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
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
			sub_ctx, scope, file,
			arg.Bucket,
			arg.Name,
			arg.CredentialsKey,
			arg.CredentialsSecret,
			arg.Region,
			arg.Endpoint,
			arg.ServerSideEncryption,
			arg.KmsEncryptionKey,
			arg.S3UploadRoot,
			arg.NoVerifyCert || arg.SkipVerify,
			uint64(stat.Size()))
		if err != nil {
			scope.Log("upload_S3: %v", err)
			// Relay the error in the UploadResponse
			return upload_response
		}
		return upload_response
	}

	return vfilter.Null{}
}

func upload_S3(ctx context.Context, scope vfilter.Scope,
	reader accessors.ReadSeekCloser,
	bucket, name string,
	credentialsKey string,
	credentialsSecret string,
	region string,
	endpoint string,
	serverSideEncryption string,
	kmsEncryptionKey string,
	s3UploadRoot string,
	NoVerifyCert bool,
	size uint64) (
	*uploads.UploadResponse, error) {

	if s3UploadRoot != "" {
		name = s3UploadRoot + name
	}
	scope.Log("upload_S3: Uploading %v to %v", name, bucket)

	conf := aws.NewConfig().WithRegion(region)
	if credentialsKey != "" && credentialsSecret != "" {
		token := ""
		creds := credentials.NewStaticCredentials(credentialsKey, credentialsSecret, token)
		_, err := creds.Get()
		if err != nil {
			return &uploads.UploadResponse{
				Error: err.Error(),
			}, err
		}

		conf = conf.WithCredentials(creds)
	}

	if endpoint != "" {
		conf = conf.WithEndpoint(endpoint).WithS3ForcePathStyle(true)

		if NoVerifyCert {
			clientConfig, _ := artifacts.GetConfig(scope)
			tlsConfig, err := networking.GetSkipVerifyTlsConfig(clientConfig)

			if err != nil {
				return &uploads.UploadResponse{
					Error: err.Error(),
				}, err
			}

			tr := &http.Transport{
				Proxy:           networking.GetProxy(),
				TLSClientConfig: tlsConfig,
				TLSNextProto: make(map[string]func(
					authority string, c *tls.Conn) http.RoundTripper),
			}

			client := &http.Client{Transport: tr}

			conf = conf.WithHTTPClient(client)
		}
	}

	sess, err := session.NewSession(conf)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	uploader := s3manager.NewUploader(sess)
	var result *s3manager.UploadOutput

	s3_params := &s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(name),
		Body:   reader,
	}
	if serverSideEncryption != "" {
		s3_params.ServerSideEncryption = aws.String(serverSideEncryption)
	}

	if kmsEncryptionKey != "" {
		s3_params.SSEKMSKeyId = aws.String(kmsEncryptionKey)
	}

	result, err = uploader.UploadWithContext(
		ctx, s3_params)

	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	// All good! report the outcome.
	response := &uploads.UploadResponse{
		Path: result.Location,
	}

	response.Size = size
	return response, nil
}

func (self S3UploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "upload_s3",
		Doc:      "Upload files to S3.",
		ArgType:  type_map.AddType(scope, &S3UploadArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&S3UploadFunction{})
}
