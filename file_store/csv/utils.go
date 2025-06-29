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
package csv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

const (
	WriteHeaders     = true
	DoNotWriteHeader = false
)

type CSVWriter struct {
	row_chan chan vfilter.Row
	wg       sync.WaitGroup
}

func (self *CSVWriter) Write(row vfilter.Row) {
	self.row_chan <- row
}

func (self *CSVWriter) Close() {
	close(self.row_chan)
	self.wg.Wait()
}

type CSVReader chan *ordereddict.Dict

func GetCSVReader(ctx context.Context, fd api.FileReader) CSVReader {
	output_chan := make(CSVReader)

	go func() {
		defer close(output_chan)

		csv_reader, err := NewReader(fd)
		if err != nil {
			return
		}

		headers, err := csv_reader.Read()
		if err != nil {
			return
		}

	process_file:
		for {
			row := ordereddict.NewDict()
			row_data, err := csv_reader.ReadAny()
			if err != nil {
				break process_file
			}

			for idx, row_item := range row_data {
				if idx > len(headers) {
					break
				}
				row.Set(headers[idx], row_item)
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan

}

func GetCSVAppender(
	config_obj *config_proto.Config,
	scope vfilter.Scope, fd io.Writer,
	write_headers bool, opts *json.EncOpts) *CSVWriter {
	result := &CSVWriter{
		row_chan: make(chan vfilter.Row),
		wg:       sync.WaitGroup{},
	}

	headers_written := true
	if write_headers {
		headers_written = false
	}

	result.wg.Add(1)
	go func() {
		defer result.wg.Done()

		w := NewWriter(fd)
		defer w.Flush()

		SetCSVOptions(config_obj, scope, w)

		columns := []string{}

		for {
			select {
			case row, ok := <-result.row_chan:
				if !ok {
					return
				}

				// First row should be the column names
				if len(columns) == 0 {
					columns = scope.GetMembers(row)
				}

				if !headers_written {
					err := w.Write(columns)
					if err != nil {
						return
					}
					headers_written = true
				}

				// We write a csv row with each cell
				// json encoded - This ensures all
				// special chars are properly escaped
				// and we can follow the csv file
				// safely.
				csv_row := []interface{}{}
				for _, column := range columns {
					item, _ := scope.Associative(row, column)
					csv_row = append(csv_row, item)
				}
				err := w.WriteAny(csv_row, opts)
				if err != nil {
					return
				}

			case <-time.After(5 * time.Second):
				w.Flush()
			}

		}

	}()

	return result
}

func GetCSVWriter(
	config_obj *config_proto.Config,
	scope vfilter.Scope, fd api.FileWriter,
	opts *json.EncOpts) (*CSVWriter, error) {
	// Seek to the end of the file.
	length, err := fd.Size()
	if err != nil {
		return nil, err
	}
	return GetCSVAppender(config_obj, scope, fd, length == 0, opts), nil
}

func EncodeToCSV(
	config_obj *config_proto.Config,
	scope vfilter.Scope, v interface{},
	opts *json.EncOpts) (string, error) {
	slice := reflect.ValueOf(v)
	if slice.Type().Kind() != reflect.Slice {
		return "", errors.New("EncodeToCSV - should be a list of rows")
	}

	buffer := &bytes.Buffer{}
	writer := GetCSVAppender(config_obj, scope, buffer, true, opts)

	for i := 0; i < slice.Len(); i++ {
		value := slice.Index(i).Interface()
		if value == nil {
			continue
		}
		writer.Write(value)
	}
	writer.Close()

	return buffer.String(), nil
}
