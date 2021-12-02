// +build !linux,!darwin,!freebsd

package utils

func CheckDirWritable(dirname string) error {
	return nil
}
