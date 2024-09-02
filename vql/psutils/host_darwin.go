//go:build darwin
// +build darwin

package psutils

import (
	"context"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func PlatformInformationWithContext(ctx context.Context) (string, string, string, error) {
	platform := ""
	family := ""
	pver := ""

	p, err := unix.Sysctl("kern.ostype")
	if err == nil {
		platform = strings.ToLower(p)
	}

	// check if the macos server version file exists
	_, err = os.Stat("/System/Library/CoreServices/ServerVersion.plist")

	// server file doesn't exist
	if os.IsNotExist(err) {
		family = "Standalone Workstation"
	} else {
		family = "Server"
	}

	return platform, family, pver, nil
}
