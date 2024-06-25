package parsers

import (
	"www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/ntfs/readers"
	vfilter "www.velocidex.com/golang/vfilter"
)

func getNTFSContextFromDevice(
	scope vfilter.Scope, device string) (
	*parser.NTFSContext, *accessors.OSPath, string, error) {
	filename, accessor, err := getOSPathAndAccessor(device)
	if err != nil {
		return nil, nil, "", err
	}

	ctx, err := readers.GetNTFSContext(scope, filename, accessor)
	return ctx, filename, accessor, err
}

func getNTFSContextFromImage(scope vfilter.Scope,
	filename *accessors.OSPath, accessor string) (*parser.NTFSContext, error) {
	return readers.GetNTFSContext(scope, filename, accessor)
}

func getNTFSContextFromMFT(scope vfilter.Scope,
	filename *accessors.OSPath, accessor string) (*parser.NTFSContext, error) {

	return readers.GetNTFSContextFromRawMFT(scope, filename, accessor)
}
