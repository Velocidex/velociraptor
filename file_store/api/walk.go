package api

import "os"

type WalkFunc func(urn FSPathSpec, info os.FileInfo) error

func Walk(
	file_store FileStore,
	root FSPathSpec,
	walkFn WalkFunc) error {
	children, err := file_store.ListDirectory(root)
	if err != nil {
		// Walking a non existant directory just ignores it.
		return nil
	}

	for _, child_info := range children {
		full_path := child_info.PathSpec()
		if child_info.IsDir() {
			err = Walk(file_store, full_path, walkFn)
			if err != nil {
				return err
			}
			continue
		}

		walkFn(full_path, child_info)
	}

	return nil
}
