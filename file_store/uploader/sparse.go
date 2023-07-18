package uploader

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *FileStoreUploader) maybeCollectSparseFile(ctx context.Context,
	reader io.Reader, store_as_name *accessors.OSPath) (*uploads.UploadResponse, error) {

	// Can the reader produce ranges?
	range_reader, ok := reader.(uploads.RangeReader)
	if !ok {
		return nil, errors.New("Not supported")
	}

	output_path := self.root_path.AddUnsafeChild(store_as_name.Components...)
	writer, err := self.file_store.WriteFile(output_path)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	sha_sum := sha256.New()
	md5_sum := md5.New()

	// The byte count we write to the output file.
	count := 0
	end_offset := int64(0)

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
			return &uploads.UploadResponse{
				Error: err.Error(),
			}, err
		}
		count += n
		end_offset = rng.Offset + int64(n)
	}

	// If there were any sparse runs, create an index.
	if is_sparse {
		writer, err := self.file_store.WriteFile(
			output_path.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX))
		if err != nil {
			return nil, err
		}
		defer writer.Close()

		serialized, err := utils.DictsToJson(index, nil)
		if err != nil {
			return &uploads.UploadResponse{
				Error: err.Error(),
			}, err
		}
		_, err = writer.Write(serialized)
		if err != nil {
			return nil, err
		}

	}

	// Return paths relative to the storage root.
	relative_path := path_specs.NewUnsafeFilestorePath(store_as_name.Components...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)

	return &uploads.UploadResponse{
		Path:       relative_path.AsClientPath(),
		Components: output_path.Components(),
		Size:       uint64(end_offset),
		StoredSize: uint64(count),
		Sha256:     hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:        hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}
