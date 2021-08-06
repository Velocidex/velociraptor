// +build deprecated

package directory

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *DirectoryFileStore) ComponentsToFileStorePath(
	components []string) string {
	sanitized_components := append(make([]string, 0, len(components)),
		self.config_obj.Datastore.FilestoreDirectory)
	for _, component := range components {
		sanitized_components = append(sanitized_components,
			utils.SanitizeString(component))
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

func (self *DirectoryFileStore) ListDirectoryComponents(components []string) (
	[]os.FileInfo, error) {

	listCounter.Inc()

	file_path := self.ComponentsToFileStorePath(components)
	files, err := utils.ReadDir(file_path)
	if err != nil {
		return nil, err
	}

	var result []os.FileInfo
	for _, fileinfo := range files {
		// Make a copy of the components for the child.
		child_components := append(
			make([]string, 0, len(components)+1), components...)
		result = append(result, &api.FileStoreFileInfo{
			FileInfo: fileinfo,
		})
	}

	return result, nil
}

func (self *DirectoryFileStore) ReadFileComponents(
	components []string) (api.FileReader, error) {
	file_path := self.ComponentsToFileStorePath(components)
	openCounter.Inc()
	file, err := os.Open(file_path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &api.FileAdapter{
		File:       file,
		Components: components,
	}, nil
}

func (self *DirectoryFileStore) StatFileComponents(
	components []string) (os.FileInfo, error) {
	file_path := self.ComponentsToFileStorePath(components)
	file, err := os.Stat(file_path)
	if err != nil {
		return nil, err
	}

	return &api.FileStoreFileInfo{
		FileInfo:   file,
		Components: components,
	}, nil
}

func (self *DirectoryFileStore) WriteFileComponent(
	components []string) (api.FileWriter, error) {
	file_path := self.ComponentsToFileStorePath(components)
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Can not create dir: %v", err)
		return nil, err
	}

	openCounter.Inc()
	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Unable to open file %v: %v", file_path, err)

		return nil, errors.WithStack(err)
	}

	return &DirectoryFileWriter{file}, nil
}
