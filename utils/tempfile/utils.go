package tempfile

import (
	"os"
	"path/filepath"
	"regexp"

	"www.velocidex.com/golang/velociraptor/utils"
)

func FindFile(tempdir string, re *regexp.Regexp) (string, error) {
	entries, err := os.ReadDir(tempdir)
	if err != nil {
		return "", err
	}

	for _, e := range entries {

		if re.MatchString(e.Name()) {
			return filepath.Join(tempdir, e.Name()), nil
		}
	}

	return "", utils.NotFoundError
}
