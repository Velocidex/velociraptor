/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
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
				string(utils.SanitizeString(component)))
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
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor string,
	store_as_name *accessors.OSPath,
	expected_size int64,
	mtime time.Time,
	atime time.Time,
	ctime time.Time,
	btime time.Time,
	mode os.FileMode,
	reader io.ReadSeeker) (*UploadResponse, error) {

	if self.UploadDir == "" {
		scope.Log("UploadDir is not set")
		return nil, errors.New("UploadDir not set")
	}

	if store_as_name == nil {
		store_as_name = filename
	}

	result, closer := DeduplicateUploads(accessor, scope, store_as_name)
	defer closer(result)
	if result != nil {
		return result, nil
	}

	file_path := self.sanitize_path(store_as_name.String())
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
	if err != nil {
		scope.Log("Can not create dir: %s(%s) %s", store_as_name.String(),
			file_path, err.Error())
		return nil, err
	}

	// For directories just create them
	if mode.IsDir() {
		err := os.MkdirAll(file_path, 0700)
		if err != nil {
			scope.Log("Can not create dir: %s(%s) %s", store_as_name.String(),
				file_path, err.Error())
			return nil, err
		}

		result = &UploadResponse{
			Path:       file_path,
			Components: store_as_name.Components,
		}
		closer(result)
		return result, nil
	}

	// Try to collect sparse files if possible
	result, err = self.maybeCollectSparseFile(
		ctx, reader, store_as_name, file_path)
	if err == nil {
		closer(result)
		return result, nil
	}

	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
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

		_, err = file.Write(data)
		if err != nil {
			return nil, err
		}

		_, err = md5_sum.Write(data)
		if err != nil {
			return nil, err
		}

		_, err = sha_sum.Write(data)
		if err != nil {
			return nil, err
		}

		offset += int64(n)
	}

	// It is not an error if we cant set the timestamps - best effort.
	err = setFileTimestamps(file_path, mtime, atime, ctime)
	if err != nil {
		scope.Log("FileBasedUploader: %v", err)
	}

	scope.Log("Uploaded %v (%v bytes)", file_path, offset)
	result = &UploadResponse{
		Path:       file_path,
		Components: store_as_name.Components,
		Size:       uint64(offset),
		Sha256:     hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:        hex.EncodeToString(md5_sum.Sum(nil)),
	}
	closer(result)
	return result, nil
}

func (self *FileBasedUploader) maybeCollectSparseFile(
	ctx context.Context,
	reader io.Reader,
	store_as_name *accessors.OSPath,
	sanitized_name string) (*UploadResponse, error) {

	// Can the reader produce ranges?
	range_reader, ok := reader.(RangeReader)
	if !ok {
		return nil, errors.New("Not supported")
	}

	writer, err := os.OpenFile(sanitized_name,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	sha_sum := sha256.New()
	md5_sum := md5.New()

	// The byte count we write to the output file.
	count := 0

	// An index array for sparse files.
	index := []*ordereddict.Dict{}
	is_sparse := false

	for _, rng := range range_reader.Ranges() {
		file_length := rng.Length
		if rng.IsSparse {
			file_length = 0
		}

		index = append(index, ordereddict.NewDict().
			Set("file_offset", count).
			Set("original_offset", rng.Offset).
			Set("file_length", file_length).
			Set("length", rng.Length))

		if rng.IsSparse {
			is_sparse = true
			continue
		}

		_, err := range_reader.Seek(rng.Offset, io.SeekStart)
		if err != nil {
			return nil, err
		}

		n, err := utils.CopyN(ctx, utils.NewTee(writer, sha_sum, md5_sum),
			range_reader, rng.Length)
		if err != nil {
			return &UploadResponse{
				Error: err.Error(),
			}, err
		}
		count += n
	}

	// If there were any sparse runs, create an index.
	if is_sparse {
		writer, err := os.OpenFile(sanitized_name+".idx",
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
		if err != nil {
			return nil, err
		}
		defer writer.Close()

		serialized, err := utils.DictsToJson(index, nil)
		if err != nil {
			return &UploadResponse{
				Error: err.Error(),
			}, err
		}
		_, err = writer.Write(serialized)
		if err != nil {
			return nil, err
		}

	}

	return &UploadResponse{
		Path:       sanitized_name,
		Components: store_as_name.Components,
		Size:       uint64(count),
		Sha256:     hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:        hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}
