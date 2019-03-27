package reporting

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_networking "www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
)

type Container struct {
	sync.Mutex // Serialize access to the zip file.
	fd         io.WriteCloser
	zip        *zip.Writer
}

func (self *Container) StoreArtifact(
	config_obj *api_proto.Config,
	ctx context.Context,
	scope *vfilter.Scope,
	vql *vfilter.VQL,
	query *actions_proto.VQLRequest) error {

	// Store the entire result set in memory because we might need
	// to re-query it when formatting the description field.
	var output_rows []vfilter.Row

	columns := vql.Columns(scope)
	for row := range vql.Eval(ctx, scope) {
		if len(*columns) == 0 {
			*columns = scope.GetMembers(row)
		}

		output_rows = append(output_rows, row)
	}

	return self.DumpRowsIntoContainer(config_obj, output_rows, scope, query)
}

func (self *Container) DumpRowsIntoContainer(
	config_obj *api_proto.Config,
	output_rows []vfilter.Row,
	scope *vfilter.Scope,
	query *actions_proto.VQLRequest) error {

	writer := ioutil.Discard

	if len(output_rows) == 0 {
		return nil
	}

	self.Lock()
	defer self.Unlock()

	sanitized_name := datastore.SanitizeString(query.Name + ".csv")
	container_writer, err := self.zip.Create(string(sanitized_name))
	if err != nil {
		return err
	}
	writer = container_writer

	csv_writer, err := csv.GetCSVWriter(scope, &StdoutWrapper{writer})
	if err != nil {
		return err
	}

	for _, row := range output_rows {
		csv_writer.Write(row)
	}

	csv_writer.Close()

	sanitized_name = datastore.SanitizeString(query.Name + ".json")
	container_writer, err = self.zip.Create(string(sanitized_name))
	if err != nil {
		return err
	}

	err = json.NewEncoder(container_writer).Encode(output_rows)
	if err != nil {
		return err
	}

	// Format the description.
	sanitized_name = datastore.SanitizeString(query.Name + ".txt")
	container_writer, err = self.zip.Create(string(sanitized_name))
	if err != nil {
		return err
	}

	fmt.Fprintf(container_writer, "# %s\n\n%s", query.Name,
		FormatDescription(config_obj, query.Description, output_rows))

	return nil
}

func (self *Container) Upload(scope *vfilter.Scope,
	filename string,
	accessor string,
	store_as_name string,
	reader io.Reader) (*vql_networking.UploadResponse, error) {
	self.Lock()
	defer self.Unlock()

	if store_as_name == "" {
		store_as_name = filename
	}

	sanitized_name := path.Join(accessor, strings.TrimLeft(store_as_name, "/\\"))
	writer, err := self.zip.Create(sanitized_name)
	if err != nil {
		return nil, err
	}

	scope.Log("Collecting file %s", store_as_name)

	sha_sum := sha256.New()
	md5_sum := md5.New()
	n, err := io.Copy(utils.NewTee(writer, sha_sum, md5_sum), reader)
	if err != nil {
		return &vql_networking.UploadResponse{
			Error: err.Error(),
		}, err
	}

	return &vql_networking.UploadResponse{
		Path:   sanitized_name,
		Size:   uint64(n),
		Sha256: hex.EncodeToString(sha_sum.Sum(nil)),
		Md5:    hex.EncodeToString(md5_sum.Sum(nil)),
	}, nil
}

func (self *Container) Close() error {
	self.zip.Close()
	return self.fd.Close()
}

func NewContainer(path string) (*Container, error) {
	fd, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}

	zip_writer := zip.NewWriter(fd)
	return &Container{
		fd:  fd,
		zip: zip_writer,
	}, nil
}

// Turns os.Stdout into into file_store.WriteSeekCloser
type StdoutWrapper struct {
	io.Writer
}

func (self *StdoutWrapper) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (self *StdoutWrapper) Close() error {
	return nil
}

func (self *StdoutWrapper) Truncate(offset int64) error {
	return nil
}
