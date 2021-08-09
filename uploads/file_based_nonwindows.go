// +build !windows

package uploads

import (
	"os"
	"time"
)

func setFileTimestamps(file_path string,
	mtime, atime, ctime time.Time) error {
	return os.Chtimes(file_path, atime, mtime)
}
