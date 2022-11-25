package accessors

import "fmt"

func ParsePath(path, path_type string) (res *OSPath, err error) {
	switch path_type {
	case "linux":
		res, err = NewLinuxOSPath(path)
	case "windows":
		res, err = NewWindowsOSPath(path)
	case "registry":
		res, err = NewWindowsRegistryPath(path)
	case "ntfs":
		res, err = NewWindowsNTFSPath(path)
	case "", "generic":
		res, err = NewGenericOSPath(path)
	case "pathspec":
		res, err = NewPathspecOSPath(path)

	case "zip":
		res, err = NewZipFilePath(path)

	default:
		err = fmt.Errorf("Unknown path type: %v (should be one of windows,linux,generic)", path_type)
	}
	return res, err
}
