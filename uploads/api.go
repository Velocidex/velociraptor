// Uploaders deliver files from accessors to the server (or another target).
package uploads

import (
	"context"
	"io"

	"www.velocidex.com/golang/vfilter"
)

// Returned as the result of the query.
type UploadResponse struct {
	Path   string `json:"Path"`
	Size   uint64 `json:"Size"`
	Error  string `json:"Error,omitempty"`
	Sha256 string `json:"sha256,omitempty"`
	Md5    string `json:"md5,omitempty"`
}

// Provide an uploader capable of uploading any reader object.
type Uploader interface {
	Upload(ctx context.Context,
		scope *vfilter.Scope,
		filename string,
		accessor string,
		store_as_name string,
		expected_size int64,
		reader io.Reader) (*UploadResponse, error)
}
