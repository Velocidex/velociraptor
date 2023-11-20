//go:build windows && cgo
// +build windows,cgo

package etw

type ProviderStat struct {
	SessionName string
	GUID        string
	Watchers    int
}
