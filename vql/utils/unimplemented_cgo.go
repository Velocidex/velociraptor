//go:build cgo

package utils

func GetMyPlatform() string {
	return _GetMyPlatform() + "_cgo"
}
