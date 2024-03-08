package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Reader struct {
	ctx        context.Context
	downloader *manager.Downloader
	offset     int64
	bucket     string
	key        string
}

func (self *S3Reader) Read(buff []byte) (int, error) {
	req := &s3.GetObjectInput{
		Bucket: aws.String(self.bucket),
		Key:    aws.String(self.key),
		Range: aws.String(
			fmt.Sprintf("bytes=%d-%d", self.offset,
				self.offset+int64(len(buff)-1))),
	}

	n, err := self.downloader.Download(self.ctx,
		manager.NewWriteAtBuffer(buff), req)

	if err != nil {
		return 0, err
	}
	self.offset += n

	return int(n), nil
}

func (self *S3Reader) Seek(offset int64, whence int) (int64, error) {
	self.offset = offset
	return self.offset, nil
}

func (self *S3Reader) Close() error {
	return nil
}
