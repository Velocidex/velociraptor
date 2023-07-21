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

	mtime_ := syscall.NsecToFiletime(mtime.UnixNano())
	atime_ := syscall.NsecToFiletime(atime.UnixNano())
	ctime_ := syscall.NsecToFiletime(ctime.UnixNano())
	err = syscall.SetFileTime(handle, &ctime_, &atime_, &mtime_)

	return err
}
