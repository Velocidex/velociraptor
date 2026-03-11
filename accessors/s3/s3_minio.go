//go:build !sumo
// +build !sumo

/* An accessor for an S3 bucket */

package s3

import (
	"context"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/minio/minio-go/v7"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Total number of keys we fetch in each ListObjects call
	mu      sync.Mutex
	maxKeys = 1000

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

	result := &RawS3SystemAccessor{
		ctx:   context.TODO(),
		scope: scope,
	}
	return result, nil
}

func (self RawS3SystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "s3",
		Description: `Allows access to S3 buckets.`,
		Permissions: []acls.ACL_PERMISSION{acls.NETWORK},
		ScopeVar:    constants.S3_CREDENTIALS,
		ArgType:     S3AcccessorArgs{},
	}
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

	s3Client, err := GetS3Client(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	if len(path.Components) == 0 {
		resp, err := s3Client.ListBuckets(self.ctx)
		if err != nil {
			return nil, err
		}
		result := make([]accessors.FileInfo, 0, len(resp))
		for _, b := range resp {
			result = append(result, &S3FileInfo{
				path:     accessors.MustNewLinuxOSPath(b.Name),
				is_dir:   true,
				mod_time: b.CreationDate,
			})
		}
		return result, nil
	}

	bucket, key, err := getBucketAndKey(path)
	if err != nil {
		return nil, err
	}

	bucket_path := accessors.MustNewLinuxOSPath(bucket)
	child_directories := ordereddict.NewDict()
	child_files := []*S3FileInfo{}

	opts := minio.ListObjectsOptions{
		Prefix:  key,
		MaxKeys: maxKeys,
	}

	obj_chan := s3Client.ListObjects(self.ctx, bucket, opts)

outer:
	for {
		select {
		case <-self.ctx.Done():
			return nil, nil

		case object, ok := <-obj_chan:
			if !ok {
				break outer
			}

			if object.Err != nil {
				continue
			}

			component_path, err := self.ParsePath(object.Key)
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
					size:     object.Size,
					mod_time: object.LastModified,
				})
			}
		}
	}

	result := []accessors.FileInfo{}
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
		return "", "", utils.NotFoundError
	}

	bucket := path.Components[0]
	components := append([]string{}, path.Components[1:]...)
	key := strings.Join(components, "/")

	return bucket, key, nil
}

func (self RawS3SystemAccessor) OpenWithOSPath(
	path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	s3Client, err := GetS3Client(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	bucket, key, err := getBucketAndKey(path)
	if err != nil {
		return nil, err
	}

	reader, err := s3Client.GetObject(
		self.ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	// Wrap the reader in an in memory cache so we do not have many
	// small reads from the network.
	paged_reader, err := utils.NewPagedReader(
		utils.MakeReaderAtter(reader), 1024*1024, 20)
	return utils.NewReadSeekReaderAdapter(paged_reader, nil), err
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

	client, err := GetS3Client(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	bucket, key, err := getBucketAndKey(path)
	if err != nil {
		return nil, err
	}

	stat_obj, err := client.StatObject(self.ctx, bucket, key,
		minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}

	return &accessors.VirtualFileInfo{
		Data_:  ordereddict.NewDict(),
		Path:   path,
		Size_:  stat_obj.Size,
		Mtime_: stat_obj.LastModified,
	}, nil
}

func init() {
	accessors.Register(&RawS3SystemAccessor{})
}

// Set the page size for tests. Normally we dont need to adjust this
// at all. Used in tests.
func SetPageSize(size int) {
	mu.Lock()
	defer mu.Unlock()

	maxKeys = size
}
