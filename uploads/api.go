// Uploaders deliver files from accessors to the server (or another target).
package uploads

import (
	"context"
	"io"
	"os"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/vfilter"
)

// Returned as the result of the query.
type UploadResponse struct {
	Path       string   `json:"Path"`
	Size       uint64   `json:"Size"`
	StoredSize uint64   `json:"StoredSize,omitempty"`
	Error      string   `json:"Error,omitempty"`
	Sha256     string   `json:"sha256,omitempty"`
	Md5        string   `json:"md5,omitempty"`
	StoredName string   `json:"StoredName,omitempty"`
	Reference  string   `json:"Reference,omitempty"`
	Components []string `json:"Components,omitempty"`
	Accessor   string   `json:"Accessor,omitempty"`

	// The type of upload this is (Currently "idx" is an index file)
	Type string `json:"Type,omitempty"`
}

// Provide an uploader capable of uploading any reader object.
type Uploader interface {
	Upload(ctx context.Context,
		scope vfilter.Scope,
		filename *accessors.OSPath,
		accessor string,
		store_as_name *accessors.OSPath,
		expected_size int64,
		mtime time.Time,
		atime time.Time,
		ctime time.Time,
		btime time.Time,
		mode os.FileMode,
		reader io.Reader) (*UploadResponse, error)
}

// A generic interface for reporting file ranges. Implementations will
// convert to this common form.

type Range struct {
	// In bytes
	Offset   int64
	Length   int64
	IsSparse bool
}

type RangeReader interface {
	io.Reader
	io.Seeker

	Ranges() []Range
}
