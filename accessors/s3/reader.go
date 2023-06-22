package s3

import (
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type S3Reader struct {
	downloader *s3manager.Downloader
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

	n, err := self.downloader.Download(aws.NewWriteAtBuffer(buff), req)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case "InvalidRange":
				// Not really an error - this happens at the end of
				// the file, just return EOF
				return 0, io.EOF
			default:
				return 0, err
			}
		}
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
