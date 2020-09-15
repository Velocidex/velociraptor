package reporting

import (
	"archive/zip"
	"bufio"
	"context"
	"io"
	"os"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/vfilter"
)

var ()

// Handle zip files written by container.go

type Archive struct {
	fd  io.WriteCloser
	zip *zip.Reader
}

func (self *Archive) openFile(name string) (io.Reader, error) {
	for _, f := range self.zip.File {
		if f.Name == name+".json" {
			return f.Open()
		}
	}

	return nil, os.ErrNotExist
}

func (self *Archive) ReadArtifactResults(
	ctx context.Context,
	scope *vfilter.Scope,
	artifact string) chan *ordereddict.Dict {

	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)

		fd, err := self.openFile(artifact)
		if err != nil {
			scope.Log("ReadArtifactResults: %v", err)
			return
		}

		reader := bufio.NewReader(fd)
		for {
			select {
			case <-ctx.Done():
				return

			default:
				row_data, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}
				item := ordereddict.NewDict()
				err = item.UnmarshalJSON(row_data)
				if err != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case output_chan <- item:
				}
			}
		}

	}()

	return output_chan
}

func (self *Archive) ListArtifacts() []string {
	result := []string{}
	seen := make(map[string]bool)
	for _, f := range self.zip.File {
		if strings.HasSuffix(f.Name, ".json") {
			name := strings.TrimSuffix(f.Name, ".json")
			artifact, _ := paths.SplitFullSourceName(name)
			_, pres := seen[artifact]
			if !pres {
				seen[artifact] = true
				result = append(result, artifact)
			}
		}
	}

	return result
}

func NewArchiveReader(path string) (*Archive, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := fd.Stat()
	if err != nil {
		return nil, err
	}

	// TODO: Handle password protected zip files.
	zip_reader, err := zip.NewReader(fd, s.Size())
	if err != nil {
		return nil, err
	}
	return &Archive{
		fd:  fd,
		zip: zip_reader,
	}, nil
}
