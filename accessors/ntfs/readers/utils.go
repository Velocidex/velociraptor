package readers

import (
	"errors"

	"www.velocidex.com/golang/velociraptor/accessors"
)

// Figure out the raw device that holds the requested NTFS filesystem.
func GetRawDeviceAndAccessor(
	device, accessor string) (string, string, error) {

	switch accessor {
	case "ntfs":
		fullpath, err := accessors.NewWindowsNTFSPath(device)
		if err != nil {
			return "", "", err
		}

		if len(fullpath.Components) == 0 {
			return "", "", errors.New("Invalid device string")
		}
		fullpath.Components = []string{fullpath.Components[0]}
		return fullpath.String(), "ntfs", nil

		// It is just an image already
	case "file":
		return device, accessor, nil

		// For raw_ntfs, the delegate contains the actual device
	case "raw_ntfs":
		fullpath, err := accessors.NewPathspecOSPath(device)
		if err != nil {
			return "", "", err
		}
		pathspec := fullpath.PathSpec()
		return pathspec.GetDelegatePath(), pathspec.GetDelegateAccessor(), nil

	default:
		return device, accessor, nil
	}
}
