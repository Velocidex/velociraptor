package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (self *CollectorTestSuite) TestDeaddisk() {
	t := self.T()

	// Create a "Windows" directory in the tmpdir
	windows_dir := filepath.Join(self.tmpdir, "Windows")
	os.MkdirAll(windows_dir, 0700)

	// Create a test file inside
	fd, err := os.OpenFile(
		filepath.Join(windows_dir, "notepad.exe"),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
	assert.NoError(t, err)

	fd.Write([]byte("Hello"))
	fd.Close()

	// Produce a remapping file.
	remapping_path := filepath.Join(self.tmpdir, "remapping.yaml")
	cmd := exec.Command(self.binary, "deaddisk", "-v",
		"--add_windows_directory", windows_dir, remapping_path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	// Now run a query on it.
	cmd = exec.Command(self.binary, "-v", "--config", remapping_path,
		"query", "SELECT Name, OSPath FROM glob(globs='C:/*')")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)

	assert.Contains(t, string(out), `"OSPath": "C:\\notepad.exe"`)
}
