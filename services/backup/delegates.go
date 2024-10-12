package backup

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type containerDelegate struct {
	*reporting.Container
	prefix string
}

func (self *containerDelegate) Create(name string, mtime time.Time) (
	io.WriteCloser, error) {
	return self.Container.Create(self.prefix+"/"+name, mtime)
}

func (self *containerDelegate) WriteResultSet(
	ctx context.Context,
	config_obj *config_proto.Config,
	dest string, in <-chan vfilter.Row) (total_rows int, err error) {

	scope := vql_subsystem.MakeScope()

	return self.Container.WriteResultSet(
		ctx, config_obj, scope, reporting.ContainerFormatJson, dest, in)
}

type zipDelegate struct {
	*zip.Reader
	prefix string
}

func (self zipDelegate) Open(name string) (fs.File, error) {
	return self.Reader.Open(self.prefix + "/" + name)
}
