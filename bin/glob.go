package main

import "path/filepath"

func expandOneGlob(path string) []string {
	res, err := filepath.Glob(path)
	if err != nil {
		return []string{path}
	}
	return res
}

// Needed for Windows as the shell does not expand globs.
func expandGlobs(paths []string) (res []string) {
	for _, p := range paths {
		res = append(res, expandOneGlob(p)...)
	}

	return res
}
