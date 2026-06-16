//go:build freebsd
// +build freebsd

package psutils

import (
	"context"

	"github.com/Velocidex/ordereddict"
)

func GetProcessDirect(ctx context.Context, pid int32) (*ordereddict.Dict, error) {
	return nil, NotImplementedError
}
