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
// A data filesystem accessor - allows data to be read as a file.

package glob

import (
	"path/filepath"
	"regexp"
	"strings"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type DataFilesystemAccessor struct{}

func (self DataFilesystemAccessor) New(scope *vfilter.Scope) (FileSystemAccessor, error) {
	return DataFilesystemAccessor{}, nil
}

func (self DataFilesystemAccessor) Lstat(filename string) (FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self DataFilesystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self DataFilesystemAccessor) Open(path string) (ReadSeekCloser, error) {
	return utils.DataReadSeekCloser{ReadSeeker: strings.NewReader(path)}, nil
}

func (self DataFilesystemAccessor) PathSplit(path string) []string {
	re := regexp.MustCompile("/")
	return re.Split(path, -1)
}

func (self DataFilesystemAccessor) PathJoin(root, stem string) string {
	return filepath.Join(root, stem)
}

func (self DataFilesystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	Register("data", &DataFilesystemAccessor{})
}
