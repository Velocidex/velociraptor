//go:build cgo

package vql

func GetMyPlatform() string {
	return _GetMyPlatform() + "_cgo"
}
