//go:build extras
// +build extras

package tools

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/Velocidex/ordereddict"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GCSUploadArgs struct {
	File        *accessors.OSPath `vfilter:"required,field=file,doc=The file to upload"`
	Name        string            `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server"`
	Accessor    string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Bucket      string            `vfilter:"required,field=bucket,doc=The bucket to upload to"`
	Project     string            `vfilter:"required,field=project,doc=The project to upload to"`
	Credentials string            `vfilter:"optional,field=credentials,doc=The credentials to use"`
}

type GCSUploadFunction struct{}

func (self *GCSUploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "upload_gcs", args)()

	arg := &GCSUploadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload_gcs: %s", err.Error())
		return vfilter.Null{}
	}

	client_config, ok := artifacts.GetConfig(scope)
	if !ok {
		scope.Log("upload_gcs: unable to fetch config")
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("ERROR:upload_gcs: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("ERROR:upload_gcs: Unable to open %s: %s",
			arg.File, err.Error())
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File.String()
	}

	stat, err := accessor.LstatWithOSPath(arg.File)
	if err != nil {
		scope.Log("ERROR:upload_gcs: Unable to stat %s: %v",
			arg.File, err)
	} else if !stat.IsDir() {
		upload_response, err := upload_gcs(
			ctx, client_config,
			scope, file, arg.Project,
			arg.Bucket,
			arg.Name, arg.Credentials)
		if err != nil {
			scope.Log("ERROR:upload_gcs: %v", err)
			return vfilter.Null{}
		}
		return upload_response
	}

	return vfilter.Null{}
}

func upload_gcs(
	ctx context.Context,
	config_obj *config_proto.ClientConfig,
	scope vfilter.Scope,
	reader io.Reader,
	projectID, bucket, name string,
	credentials string) (
	*uploads.UploadResponse, error) {

	// Cache the bucket handle between invocations.
	var bucket_handle *storage.BucketHandle
	bucket_handle_cache := vql_subsystem.CacheGet(scope, bucket)
	if bucket_handle_cache == nil {
		http_transport, err := networking.GetHttpTransport(config_obj, "")
		if err != nil {
			return nil, err
		}

		http_transport = networking.MaybeSpyOnTransport(
			&config_proto.Config{Client: config_obj}, http_transport)

		var creds *google.Credentials
		if len(credentials) > 0 {
			creds, err = transport.Creds(ctx,
				option.WithAuthCredentialsJSON(option.ServiceAccount, []byte(credentials)),
				option.WithScopes(storage.ScopeReadWrite))
		} else {
			creds, err = google.FindDefaultCredentials(ctx)
		}
		if err != nil {
			return nil, err
		}

		t_http_client := &http.Client{
			Transport: &oauth2.Transport{
				Base:   http_transport,
				Source: creds.TokenSource,
			},
		}

		// Theoretically option.WithCredentialsJSON can be provided to
		// storage.NewClient but this is currently broken upstream. We
		// add the credentials to the transport directly instead.
		// https://github.com/googleapis/google-api-go-client/issues/3414
		client, err := storage.NewClient(ctx,
			option.WithHTTPClient(t_http_client))
		if err != nil {
			return nil, err
		}

		bucket_handle = client.Bucket(bucket)
		vql_subsystem.CacheSet(scope, bucket, bucket_handle)

	} else {
		bucket_handle = bucket_handle_cache.(*storage.BucketHandle)
	}

	scope.Log("upload_gcs: Uploading %v to %v", name, bucket)
	obj := bucket_handle.Object(name)
	writer := obj.NewWriter(ctx)

	sha_sum := sha256.New()
	md5_sum := md5.New()

	defer func() {
		err := writer.Close()
		if err != nil {
			scope.Log("ERROR:upload_gcs: <red>ERROR writing to object: %v", err)
		} else {
			attr := writer.Attrs()
			serialized, _ := json.Marshal(attr)
			scope.Log("upload_gcs: SUCCESS writing to object: %v",
				string(serialized))

			if string(attr.MD5) == string(md5_sum.Sum(nil)) {
				scope.Log(
					"DEBUG: upload_gcs: <red>GCS Calculated MD5: %016x Hash checks out.",
					attr.MD5)

			} else {
				scope.Log(
					"ERROR: upload_gcs: <red>GCS Calculated MD5: %016x Hash mismatch!!!",
					attr.MD5)
			}
		}
	}()

	log_writer := &vql_subsystem.LogWriter{
		Scope:   scope,
		Message: "upload_gcs " + name}

	n, err := utils.Copy(ctx, utils.NewTee(
		writer, sha_sum, md5_sum, log_writer), reader)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	return &uploads.UploadResponse{
		Path:   name,
		Size:   uint64(n),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}

func (self GCSUploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "upload_gcs",
		Doc:      "Upload files to GCS.",
		ArgType:  type_map.AddType(scope, &GCSUploadArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GCSUploadFunction{})
}
