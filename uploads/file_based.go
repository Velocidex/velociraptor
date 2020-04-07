/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package uploads

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type FileBasedUploader struct {
	UploadDir string
}

// Turn the path which may have a device name into something which can
// be created as a directory.
var sanitize_re = regexp.MustCompile(
	`\\\\[\\.\\?]\\([{}a-zA-Z0-9]+).*?\\`)

func (self *FileBasedUploader) sanitize_path(path string) string {
	// Strip any leading devices, and make sure the device name
	// consists of valid chars.
	path = sanitize_re.ReplaceAllString(path, `$1\`)

	// Split the path into components and escape any non valid
	// chars.
	components := []string{self.UploadDir}
	for _, component := range utils.SplitComponents(path) {
		if len(component) > 0 {
			components = append(components,
				string(datastore.SanitizeString(component)))
		}
	}

	result := filepath.Join(components...)
	if runtime.GOOS == "windows" {
		result, _ = filepath.Abs(result)
		return "\\\\?\\" + result
	}

	return result
}

func (self *FileBasedUploader) Upload(
	ctx context.Context,
	scope *vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	expected_size int64,
	reader io.Reader) (
	*UploadResponse, error) {

	if self.UploadDir == "" {
		scope.Log("UploadDir is not set")
		return nil, errors.New("UploadDir not set")
	}

	if store_as_name == "" {
		store_as_name = filename
	}

	file_path := self.sanitize_path(store_as_name)
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
	if err != nil {
		scope.Log("Can not create dir: %s(%s) %s", store_as_name,
			file_path, err.Error())
		return nil, err
	}

	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		scope.Log("Unable to open file %s: %s", file_path, err.Error())
		return nil, err
	}
	defer file.Close()

	buf := make([]byte, 1024*1024)
	offset := int64(0)
	md5_sum := md5.New()
	sha_sum := sha256.New()

	for {
		n, _ := reader.Read(buf)
		if n == 0 {
			break
		}
		data := buf[:n]

		file.Write(data)
		md5_sum.Write(data)
		sha_sum.Write(data)

		offset += int64(n)
	}

	scope.Log("Uploaded %v (%v bytes)", file_path, offset)
	return &UploadResponse{
		Path:   file_path,
		Size:   uint64(offset),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}
