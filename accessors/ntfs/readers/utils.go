package readers

import (
	"errors"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/vfilter"
)

// Figure out the raw device that holds the requested NTFS filesystem.
func GetRawDeviceAndAccessor(
	scope vfilter.Scope,
	device *accessors.OSPath, accessor string) (*accessors.OSPath, string, error) {

	switch accessor {
	case "ntfs":
		if len(device.Components) == 0 {
			return nil, "", errors.New("Invalid device string")
		}
		return device.Clear().Append(device.Components[0]), "ntfs", nil

		// It is just an image already
	case "file":
		return device, accessor, nil

		// For raw_ntfs, the delegate contains the actual device
	case "raw_ntfs":
		delegate, err := device.Delegate(scope)
		return delegate, device.DelegateAccessor(), err

	default:
		return device, accessor, nil
	}
}
