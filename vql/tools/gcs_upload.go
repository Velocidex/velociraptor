package tools

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
)

type GCSUploadArgs struct {
	File        string `vfilter:"required,field=file,doc=The file to upload"`
	Name        string `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server"`
	Accessor    string `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Bucket      string `vfilter:"required,field=bucket,doc=The bucket to upload to"`
	Project     string `vfilter:"required,field=project,doc=The project to upload to"`
	Credentials string `vfilter:"required,field=credentials,doc=The credentials to use"`
}

type GCSUploadFunction struct{}

func (self *GCSUploadFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {

	arg := &GCSUploadArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("upload_gcs: %s", err.Error())
		return vfilter.Null{}
	}

	accessor, err := glob.GetAccessor(arg.Accessor, ctx)
	if err != nil {
		scope.Log("upload_gcs: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.Open(arg.File)
	if err != nil {
		scope.Log("upload_gcs: Unable to open %s: %s",
			arg.File, err.Error())
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File
	}

	stat, err := file.Stat()
	if err != nil {
		scope.Log("upload_gcs: Unable to stat %s: %v",
			arg.File, err)
	} else if !stat.IsDir() {
		upload_response, err := upload_gcs(
			ctx, scope, file, arg.Project,
			arg.Bucket,
			arg.Name, arg.Credentials)
		if err != nil {
			scope.Log("upload_gcs: %v", err)
			return vfilter.Null{}
		}
		return upload_response
	}

	return vfilter.Null{}
}

func objectURL(objAttrs *storage.ObjectAttrs) string {
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s",
		objAttrs.Bucket, objAttrs.Name)
}

func upload_gcs(ctx context.Context, scope *vfilter.Scope,
	reader io.Reader,
	projectID, bucket, name string,
	credentials string) (
	*networking.UploadResponse, error) {

	// Cache the bucket handle between invocations.
	var bucket_handle *storage.BucketHandle
	bucket_handle_cache := vql_subsystem.CacheGet(scope, bucket)
	if bucket_handle_cache == nil {
		client, err := storage.NewClient(ctx, option.WithCredentialsJSON(
			[]byte(credentials)))
		if err != nil {
			return nil, err
		}

		bucket_handle = client.Bucket(bucket)
		vql_subsystem.CacheSet(scope, bucket, bucket_handle)

	} else {
		bucket_handle = bucket_handle_cache.(*storage.BucketHandle)
	}

	obj := bucket_handle.Object(name)
	writer := obj.NewWriter(ctx)
	defer writer.Close()

	sha_sum := sha256.New()
	md5_sum := md5.New()
	log_writer := &vql_subsystem.LogWriter{
		Scope:   scope,
		Message: "upload_gcs " + name}

	n, err := utils.Copy(ctx, utils.NewTee(
		writer, sha_sum, md5_sum, log_writer), reader)
	if err != nil {
		return &networking.UploadResponse{
			Error: err.Error(),
		}, err
	}

	return &networking.UploadResponse{
		Path:   name,
		Size:   uint64(n),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}

func (self GCSUploadFunction) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "upload_gcs",
		Doc:     "Upload files to GCS.",
		ArgType: type_map.AddType(scope, &GCSUploadFunction{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GCSUploadFunction{})
}
