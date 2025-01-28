package api

import (
	"os"

	"github.com/go-errors/errors"
)

var (
	STOP_ITERATION = errors.New("Stop Iteration")
)

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

		err = walkFn(full_path, child_info)
		if err != nil {
			return err
		}
	}

	return nil
}

func RecursiveDelete(
	file_store FileStore, root FSPathSpec) error {
	return Walk(file_store, root,
		func(path FSPathSpec, info os.FileInfo) error {
			// Ignore errors so we can keep going as much as possible.
			_ = file_store.Delete(path)
			return nil
		})
}
