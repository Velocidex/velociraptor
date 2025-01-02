package reporting

import (
	"os"

	concurrent_zip "github.com/Velocidex/zip"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
)

type TmpfileFactory int

func (self TmpfileFactory) TempFile() (*os.File, error) {
	tmpfile, err := tempfile.TempFile("zip")
	if err != nil {
		return nil, err
	}
	tempfile.AddTmpFile(tmpfile.Name())

	return tmpfile, nil
}

func (self TmpfileFactory) RemoveTempFile(filename string) {
	err := os.Remove(filename)
	tempfile.RemoveTmpFile(filename, err)
}

func init() {
	concurrent_zip.SetTmpfileProvider(TmpfileFactory(0))
}
