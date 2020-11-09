package recyclebin

import (
	"io"
)

type FileInfo struct {
	FileSize       uint64 `json:"FileSize"`
	DeletedTime    uint64 `json:"DeletedTime"`
	FileNameLength uint32 `json:"FileNameLength"`
	FilePath       string `json:"FilePath"`
}

func ParseRecycleBin(reader io.ReaderAt) (*FileInfo, error) {
	profile := NewRecycleBinIndex()
	meta := profile.Metadata(reader, 0)

	self := &FileInfo{
		FileSize:       meta.FileSize(),
		DeletedTime:    meta.DeletedTime(),
		FileNameLength: meta.FileNameLength(),
		FilePath:       meta.FilePath(),
	}

	return self, nil
}
