/* An accessor for an S3 bucket */

package s3

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Total number of keys we fetch in each ListObjects call
	mu      sync.Mutex
	maxKeys = int32(1000)

	metricS3OpsListObjects = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "s3_ops_list_objects",
			Help: "Number of s3 ListObjects operations",
		})
)

type RawS3SystemAccessor struct {
	ctx   context.Context
	scope vfilter.Scope
}

func (self RawS3SystemAccessor) ParsePath(path string) (*accessors.OSPath, error) {
	return accessors.NewLinuxOSPath(path)
}

func (self RawS3SystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	// Check we have permission to open files.
	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_READ)
	if err != nil {
		return nil, err
	}

	result := &RawS3SystemAccessor{
		ctx:   context.TODO(),
		scope: scope,
	}
	return result, nil
}

func (self RawS3SystemAccessor) ReadDir(
	path string) ([]accessors.FileInfo, error) {

	parsed_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(parsed_path)
}

func (self RawS3SystemAccessor) ReadDirWithOSPath(
	path *accessors.OSPath) ([]accessors.FileInfo, error) {

	client, err := GetS3Client(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	if len(path.Components) == 0 {
		resp, err := client.ListBuckets(self.ctx, &s3.ListBucketsInput{})
		if err != nil {
			return nil, err
		}
		result := make([]accessors.FileInfo, 0, len(resp.Buckets))
		for _, b := range resp.Buckets {
			result = append(result, &S3FileInfo{
				path:     accessors.MustNewLinuxOSPath(*b.Name),
				is_dir:   true,
				mod_time: *b.CreationDate,
			})
		}
		return result, nil
	}

	bucket, key, err := getBucketAndKey(path)
	if err != nil {
		return nil, err
	}

	// Keys may not have a leading / but we should handle them as
	// well.
	key = strings.TrimPrefix(key, "/")
	bucket_path := accessors.MustNewLinuxOSPath(bucket)
	child_directories := ordereddict.NewDict()
	child_files := []*S3FileInfo{}

	params := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(path.Dirname().String()),
	}

	// Create the Paginator for the ListObjectsV2 operation.
	paginator := s3.NewListObjectsV2Paginator(
		client, params, func(o *s3.ListObjectsV2PaginatorOptions) {
			mu.Lock()
			defer mu.Unlock()

			o.Limit = maxKeys
		})

	result := []accessors.FileInfo{}
	for paginator.HasMorePages() {
		metricS3OpsListObjects.Inc()

		page, err := paginator.NextPage(self.ctx)
		if err != nil {
			return nil, err
		}

		for _, object := range page.Contents {
			component_path, err := self.ParsePath(*object.Key)
			if err != nil {
				continue
			}

			object_path := bucket_path.Append(component_path.Components...)

			// Skip components that are not direct children.
			if len(object_path.Components) > len(path.Components)+1 {
				child_directories.Set(
					object_path.Components[len(path.Components)], true)

			} else if len(object_path.Components) == len(path.Components)+1 {
				child_files = append(child_files, &S3FileInfo{
					path:     object_path,
					is_dir:   false,
					size:     *object.Size,
					mod_time: *object.LastModified,
				})
			}
		}
	}

	for _, child_dir := range child_directories.Keys() {
		result = append(result, &S3FileInfo{
			path:   path.Append(child_dir),
			is_dir: true,
		})
	}

	for _, info := range child_files {
		result = append(result, info)
	}

	return result, nil
}

func getBucketAndKey(path *accessors.OSPath) (string, string, error) {
	if len(path.Components) == 0 {
		return "", "", os.ErrNotExist
	}

	bucket := path.Components[0]
	components := append([]string{}, path.Components[1:]...)
	key := "/" + strings.Join(components, "/")

	return bucket, key, nil
}

func (self RawS3SystemAccessor) OpenWithOSPath(
	path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	svc, err := GetS3Client(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	bucket, key, err := getBucketAndKey(path)
	if err != nil {
		return nil, err
	}

	reader := &S3Reader{
		ctx:        self.ctx,
		downloader: manager.NewDownloader(svc),
		bucket:     bucket,
		key:        key,
	}

	// Wrap the reader in an in memory cache so we do not have many
	// small reads from the network.
	paged_reader, err := utils.NewPagedReader(
		utils.MakeReaderAtter(reader), 1024*1024, 20)
	return utils.NewReadSeekReaderAdapter(paged_reader), err
}

func (self RawS3SystemAccessor) Open(
	filename string) (accessors.ReadSeekCloser, error) {

	parsed_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(parsed_path)
}

func (self RawS3SystemAccessor) Lstat(path string) (accessors.FileInfo, error) {

	parsed_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(parsed_path)
}

func (self RawS3SystemAccessor) LstatWithOSPath(
	path *accessors.OSPath) (accessors.FileInfo, error) {

	svc, err := GetS3Client(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	bucket, key, err := getBucketAndKey(path)
	if err != nil {
		return nil, err
	}

	headObj := s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := svc.HeadObject(self.ctx, &headObj)
	if err != nil {
		return nil, err
	}

	return &accessors.VirtualFileInfo{
		Data_: ordereddict.NewDict(),
		Path:  path,
		Size_: *result.ContentLength,
	}, nil
}

func init() {
	accessors.Register("s3", &RawS3SystemAccessor{},
		`Access S3 Buckets.

This artifact allows access to S3 buckets:

1. The first component is interpreted as the bucket name.

2. Provide credentials through the VQL environment
   variable S3_CREDENTIALS. This should be a dict with
   a key of the bucket name and the value being the credentials.

Example:

LET S3_CREDENTIALS<=dict(endpoint='http://127.0.0.1:4566/',
  credentials_key='admin',
  credentials_secret='password',
  no_verify_cert=1)

SELECT *, read_file(filename=OSPath,
   length=10, accessor='s3') AS Data
FROM glob(globs='/velociraptor/orgs/root/clients/C.39a107c4c58c5efa/collections/*/uploads/auto/*', accessor='s3')

`)
}

// Set the page size for tests. Normally we dont need to adjust this
// at all. Used in tests.
func SetPageSize(size int32) {
	mu.Lock()
	defer mu.Unlock()

	maxKeys = size
}
