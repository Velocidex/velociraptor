package materializer

// Test: materialize.in.yaml

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	utils_tempfile "www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/filesystem"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/materializer"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	_Materialize_error_msg = "ERROR:Unable to create tempfile for expansion of LET %v: %v, giving up after %v rows"
	_Materialize_info_msg  = "WARN:Materialize of LET %s: Expand larger than %v rows, VQL will switch to tempfile backing on %v which will be much slower."
)

// A materializer that works off a disk file.
type TempFileMatrializer struct {
	filename string
	tempfile io.Closer
	writer   *bufio.Writer
}

func NewTempFileMatrializer(
	ctx context.Context, scope types.Scope,
	name string, rows []types.Row) (*TempFileMatrializer, error) {

	// name is a VQL identifier so should be safe.
	tmpfile, err := tempfile.TempFile(
		"VQL_" + utils.SanitizeString(name) + "_.jsonl")
	if err != nil {
		return nil, err
	}
	utils_tempfile.AddTmpFile(tmpfile.Name())

	root_scope := vql_subsystem.GetRootScope(scope)
	err = root_scope.AddDestructor(func() {
		filesystem.RemoveTmpFile(0, tmpfile.Name(), root_scope)
	})
	if err != nil {
		return nil, err
	}

	result := &TempFileMatrializer{
		filename: tmpfile.Name(),
		tempfile: tmpfile,
		writer:   bufio.NewWriter(tmpfile),
	}

	// Now just dump the rows into the file
	for _, row := range rows {
		err := result.WriteRow(row)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (self *TempFileMatrializer) Materialize(
	ctx context.Context, scope types.Scope) types.Any {
	return &types.Null{}
}

func (self *TempFileMatrializer) Close() {
	self.writer.Flush()
	self.tempfile.Close()
}

func (self *TempFileMatrializer) WriteRow(row types.Row) error {
	serialized, err := json.Marshal(row)
	if err != nil {
		return err
	}

	_, err = self.writer.Write(serialized)
	if err != nil {
		return err
	}

	return self.writer.WriteByte('\n')
}

// Support StoredQuery protocol.
func (self TempFileMatrializer) Eval(
	ctx context.Context, scope types.Scope) <-chan types.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		fd, err := os.Open(self.filename)
		if err != nil {
			scope.Log("Unable to open file %s: %v", self.filename, err)
			return
		}
		defer fd.Close()

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

// Support Associative protocol
func (self TempFileMatrializer) Applicable(a types.Any, b types.Any) bool {
	_, ok := a.(*TempFileMatrializer)
	if !ok {
		return false
	}

	return true
}

// Just deletegate to our contained rows array.
func (self TempFileMatrializer) GetMembers(
	scope types.Scope, a types.Any) []string {
	return nil
}

// We do not support Associative in the regular sense because this
// might create huge arrays. Normally a query like this is allowed:
// LET X <= SELECT Pid, Name FROM pslist()
// SELECT ... X.Name AS AllProcessNames ...
//
// Where VQL expands X.Name into an in memory array with the Name
// column in each item over all the rows.  When dealing with file
// backed queries this can result in a massive in-memory array and so
// we just refuse it. For small enough materialized queries, the in
// memory materializer will be used so this will still work for
// queries with a small number of rows but not work at all for larger
// number.
func (self TempFileMatrializer) Associative(
	scope types.Scope, a types.Any, b types.Any) (res types.Any, pres bool) {
	return &vfilter.Null{}, false
}

// It makes no sense to marshal the entire file into a string so we
// deliberately ignore it here. For example this query.
// LET X <= SELECT ...
// SELECT X FROM scope()

// Typically marshalling a stored query is not really encouraged
// because it expands the entire query into a single cell, but will
// still work for smaller queries which use the in-memory
// materializer.
func (self *TempFileMatrializer) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

// An object implementing the ScopeMaterializer interface. This
// materializer backs the data into memory until reading the limit and
// then stores the data in a temp file on disk transparently.  You can
// control the limit of the materialized threashold by setting the
// VQL_MATERIALIZE_ROW_LIMIT variable (default is 1000 rows)
type Materializer struct{}

func (self Materializer) Materialize(
	ctx context.Context, scope types.Scope,
	name string, query types.StoredQuery) types.StoredQuery {

	row_limit := vql_subsystem.GetIntFromRow(
		scope, scope, constants.VQL_MATERIALIZE_ROW_LIMIT)
	if row_limit == 0 {
		row_limit = 1000
	}

	rows := []types.Row{}
	var file_writer *TempFileMatrializer
	var err error

	for row := range query.Eval(ctx, scope) {
		if file_writer != nil {
			err := file_writer.WriteRow(row)
			if err != nil {
				scope.Log(_Materialize_error_msg, name, err, len(rows))
				return materializer.NewInMemoryMatrializer(rows)
			}

		} else {
			rows = append(rows, row)

			if uint64(len(rows)) > row_limit {
				file_writer, err = NewTempFileMatrializer(ctx, scope,
					name, rows)
				if err != nil {
					// If we are unable to create a file backing we
					// stop evaluating early and return partial
					// results. This is safer than just consuming all
					// memory.
					scope.Log(_Materialize_error_msg, name, err, len(rows))
					return materializer.NewInMemoryMatrializer(rows)
				}
				defer file_writer.Close()

				scope.Log(_Materialize_info_msg, name,
					row_limit, file_writer.filename)
				rows = nil
			}
		}
	}

	if file_writer != nil {
		return file_writer
	}
	return materializer.NewInMemoryMatrializer(rows)
}

func NewMaterializer() *Materializer {
	return &Materializer{}
}

func init() {
	vql_subsystem.RegisterProtocol(&TempFileMatrializer{})
}
