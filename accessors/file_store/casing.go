package file_store

import (
	"strings"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Correct the filename to its correct casing.
func getCorrectCase(
	file_store api.FileStore,
	filename api.FSPathSpec) (api.FSPathSpec, error) {

	// File is exactly fine.
	_, err := file_store.StatFile(filename)
	if err == nil {
		return filename, nil
	}

	// File is not found, try to find the correct casing for the base
	// component.
	basename := filename.Base()
	dirname := filename.Dir()

	// For non root directories we need to look at the parents
	if len(dirname.Components()) > 0 {
		_, err := file_store.StatFile(dirname)
		if err != nil {
			// The parent directory can not be directly opened - it is
			// possible that the parent directory casing is incorrect too.
			dirname, err = getCorrectCase(file_store, dirname)
			if err != nil {
				return nil, err
			}
		}
	}

	// Try again with the correct case. It should work this time.
	entries, err := file_store.ListDirectory(dirname)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if strings.EqualFold(e.Name(), basename) {
			return dirname.AddChild(e.Name()), nil
		}
	}

	return nil, utils.NotFoundError
}
