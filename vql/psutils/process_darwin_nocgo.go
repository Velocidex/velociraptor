//go:build darwin && !cgo
// +build darwin,!cgo

package psutils

import (
	"context"
)

func cmdNameWithContext(ctx context.Context, pid int32) (string, error) {
	return "", NotImplementedError
}

func cmdlineSliceWithContext(ctx context.Context, pid int32) ([]string, error) {
	return nil, NotImplementedError
}

func TimesWithContext(ctx context.Context, pid int32) (*TimesStat, error) {
	return nil, NotImplementedError
}

func ExeWithContext(ctx context.Context, pid int32) (string, error) {
	return "", NotImplementedError
}

func CwdWithContext(ctx context.Context, pid int32) (string, error) {
	return "", NotImplementedError
}

func MemoryInfoWithContext(ctx context.Context, pid int32) (*MemoryInfoStat, error) {
	return nil, NotImplementedError
}
