//+build extras

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
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type S3UploadArgs struct {
	File                 string `vfilter:"required,field=file,doc=The file to upload"`
	Name                 string `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server"`
	Accessor             string `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Bucket               string `vfilter:"required,field=bucket,doc=The bucket to upload to"`
	Region               string `vfilter:"required,field=region,doc=The region the bucket is in"`
	CredentialsKey       string `vfilter:"required,field=credentialskey,doc=The AWS key credentials to use"`
	CredentialsSecret    string `vfilter:"required,field=credentialssecret,doc=The AWS secret credentials to use"`
	Endpoint             string `vfilter:"optional,field=endpoint,doc=The Endpoint to use"`
	ServerSideEncryption string `vfilter:"optional,field=serversideencryption,doc=The server side encryption method to use"`
	NoVerifyCert         bool   `vfilter:"optional,field=noverifycert,doc=Skip TLS Verification"`
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

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("upload_S3: %s", err)
		return vfilter.Null{}
	}

	accessor, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_S3: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.Open(arg.File)
	if err != nil {
		scope.Log("upload_S3: Unable to open %s: %s",
			arg.File, err.Error())
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File
	}

	stat, err := file.Stat()
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
			arg.NoVerifyCert)
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
	reader glob.ReadSeekCloser,
	bucket, name string,
	credentialsKey string,
	credentialsSecret string,
	region string,
	endpoint string,
	serverSideEncryption string,
	NoVerifyCert bool) (
	*api.UploadResponse, error) {

	scope.Log("upload_S3: Uploading %v to %v", name, bucket)

	token := ""
	creds := credentials.NewStaticCredentials(credentialsKey, credentialsSecret, token)
	_, err := creds.Get()
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	conf := aws.NewConfig().WithRegion(region).WithCredentials(creds)
	if endpoint != "" {
		conf = conf.WithEndpoint(endpoint).WithS3ForcePathStyle(true)
		if NoVerifyCert {
			tr := &http.Transport{
				Proxy:           http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}

			client := &http.Client{Transport: tr}

			conf = conf.WithHTTPClient(client)
		}

	}
	sess, err := session.NewSession(conf)
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	uploader := s3manager.NewUploader(sess)
	var result *s3manager.UploadOutput
	if serverSideEncryption != "" {
		result, err = uploader.UploadWithContext(
			ctx, &s3manager.UploadInput{
				Bucket:               aws.String(bucket),
				Key:                  aws.String(name),
				ServerSideEncryption: aws.String(serverSideEncryption),
				Body:                 reader,
			})
	} else {
		result, err = uploader.UploadWithContext(
			ctx, &s3manager.UploadInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(name),
				Body:   reader,
			})
	}
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	// All good! report the outcome.
	response := &api.UploadResponse{
		Path: result.Location,
	}

	stat, err := reader.Stat()
	if err == nil {
		response.Size = uint64(stat.Size())
	}

	return response, nil
}

func (self S3UploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "upload_s3",
		Doc:     "Upload files to S3.",
		ArgType: type_map.AddType(scope, &S3UploadArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&S3UploadFunction{})
}
