package s3

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
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
	to_read := int64(len(buff)) - 1

	req := &s3.GetObjectInput{
		Bucket: aws.String(self.bucket),
		Key:    aws.String(self.key),
		Range: aws.String(
			fmt.Sprintf("bytes=%d-%d", self.offset,
				self.offset+to_read)),
	}

	n, err := self.downloader.Download(self.ctx,
		manager.NewWriteAtBuffer(buff), req)

	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) {
			if re.HTTPStatusCode() == 416 {
				return 0, io.EOF
			}
		}

		return 0, err
	}
	self.offset += n

	if n < to_read {
		return int(n), io.EOF
	}

	return int(n), nil
}

func (self *S3Reader) Seek(offset int64, whence int) (int64, error) {
	self.offset = offset
	return self.offset, nil
}

func (self *S3Reader) Close() error {
	return nil
}
