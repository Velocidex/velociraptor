// +build windows

package uploads

import (
	"syscall"
	"time"
)

func setFileTimestamps(file_path string,
	mtime, atime, ctime time.Time) error {
	pathp, err := syscall.UTF16PtrFromString(file_path)
	if err != nil {
		return err
	}

	handle, err := syscall.CreateFile(pathp,
		syscall.FILE_WRITE_ATTRIBUTES, syscall.FILE_SHARE_WRITE, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(handle)

	mtime := syscall.NsecToFiletime(mtime.UnixNano())
	atime := syscall.NsecToFiletime(mtime.UnixNano())
	ctime := syscall.NsecToFiletime(mtime.UnixNano())
	return syscall.SetFileTime(h, &ctime, &atime, &mtime)
}
