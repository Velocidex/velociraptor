package fuse

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
)

type Timestamps struct {
	Mtime    time.Time `json:"Modified"`
	Atime    time.Time `json:"LastAccessed"`
	Ctime    time.Time `json:"Changed"`
	Btime    time.Time `json:"Created"`
	Filename string    `json:"SourceFile"`
}

var (
	knownMetadataFiles = [][]string{
		// 0.74 and below
		{"results", "Windows.KapeFiles.Targets/All File Metadata.json"},

		// 0.75 +
		{"results", "Windows.KapeFiles.Targets/All Matches Metadata.json"},
		{"results", "Windows.Triage.Targets/All Matches Metadata.json"},
	}
)

func (self *Options) getTimestamp(filename *accessors.OSPath) (*Timestamps, bool) {
	components := filename.Components
	if len(components) < 3 {
		return nil, false
	}

	components = components[2:]
	windows_path := strings.Join(components, "\\")

	self.mu.Lock()
	defer self.mu.Unlock()

	hit, ok := self.timestamps[windows_path]

	return hit, ok
}

func (self *Options) parseTimestamps(
	accessor accessors.FileSystemAccessor,
	filename *accessors.OSPath) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if !self.EmulateTimestamps {
		return
	}

	if self.timestamps == nil {
		self.timestamps = make(map[string]*Timestamps)
	}

	// Try to find the results from the known artifacts
	for _, components := range knownMetadataFiles {
		fd, err := accessor.OpenWithOSPath(filename.Append(components...))
		if err != nil {
			continue
		}
		defer fd.Close()

		reader := bufio.NewReader(fd)
		for {
			row_data, err := reader.ReadBytes('\n')
			if len(row_data) == 0 || (err != nil && !errors.Is(err, io.EOF)) {
				break
			}

			// Skip empty lines
			if len(row_data) == 1 {
				continue
			}

			timestamp := &Timestamps{}
			err = json.Unmarshal(row_data, timestamp)
			if err != nil {
				continue
			}

			self.timestamps[timestamp.Filename] = timestamp
		}
	}

}
