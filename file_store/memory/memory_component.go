package memory

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

func (self *MemoryFileStore) ComponentsToFileStorePath(
	components []string) string {
	sanitized_components := make([]string, 0, len(components))
	for _, component := range components {
		sanitized_components = append(sanitized_components,
			string(utils.SanitizeString(component)))
	}

	// OS filenames may use / or \ as separators. On windows we
	// prefix the LFN prefix to be able to access long paths but
	// then we must use \ as a separator.
	result := filepath.Join(sanitized_components...)

	if runtime.GOOS == "windows" {
		return WINDOWS_LFN_PREFIX + result
	}
	return result
}

func (self *MemoryFileStore) ReadFileComponents(
	components []string) (api.FileReader, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := self.ComponentsToFileStorePath(components)
	data_any, pres := self.Data.Get(filename)
	if pres {
		data := data_any.([]byte)
		return MemoryReader{
			Reader:   bytes.NewReader(data),
			filename: filename,
		}, nil
	}

	return nil, errors.New("Not found")
}

func (self *MemoryFileStore) WriteFileComponent(
	components []string) (api.FileWriter, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := self.ComponentsToFileStorePath(components)
	buf, pres := self.Data.Get(filename)
	if !pres {
		buf = []byte{}
	}
	self.Data.Set(filename, buf)

	return &MemoryWriter{
		buf:               buf.([]byte),
		memory_file_store: self,
		filename:          filename,
	}, nil
}

func (self *MemoryFileStore) StatFileComponents(
	components []string) (os.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := self.ComponentsToFileStorePath(components)
	buff, pres := self.Data.Get(filename)
	if !pres {
		return nil, os.ErrNotExist
	}

	return &vtesting.MockFileInfo{
		Name_:     path.Base(filename),
		FullPath_: filename,
		Size_:     int64(len(buff.([]byte))),
	}, nil
}

func (self *MemoryFileStore) ListDirectoryComponents(
	path_components []string) ([]os.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	dirname := self.ComponentsToFileStorePath(path_components)
	result := []os.FileInfo{}
	files := []string{}
	for _, filename := range self.Data.Keys() {
		v_any, _ := self.Data.Get(filename)
		v := v_any.([]byte)

		if strings.HasPrefix(filename, dirname) {
			k := strings.TrimLeft(
				strings.TrimPrefix(filename, dirname), "/")
			components := strings.Split(k, "/")
			if len(components) > 0 &&
				!utils.InString(files, components[0]) {
				new_components := append(
					make([]string, 0, len(path_components)),
					path_components...)

				result = append(result, &vtesting.MockFileInfo{
					Name_: components[0],
					// Make a new copy for each child.
					Components: append(new_components,
						components[0]),
					Size_: int64(len(v)),
				})
				files = append(files, components[0])
			}
		}
	}
	return result, nil
}
