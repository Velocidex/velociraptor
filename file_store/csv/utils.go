package csv

import (
	"os"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/vfilter"
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

type CSVReader chan *vfilter.Dict

func GetCSVReader(fd file_store.ReadSeekCloser) CSVReader {
	output_chan := make(CSVReader)

	go func() {
		defer close(output_chan)

		csv_reader := NewReader(fd)
		headers, err := csv_reader.Read()
		if err != nil {
			return
		}

	process_file:
		for {
			row := vfilter.NewDict()
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

			output_chan <- row
		}
	}()

	return output_chan

}

func GetCSVWriter(scope *vfilter.Scope, fd file_store.WriteSeekCloser) (*CSVWriter, error) {
	result := &CSVWriter{
		row_chan: make(chan vfilter.Row),
		wg:       sync.WaitGroup{},
	}

	// Seek to the end of the file.
	length, err := fd.Seek(0, os.SEEK_END)
	if err != nil {
		return nil, err
	}
	headers_written := length > 0

	go func() {
		result.wg.Add(1)
		defer result.wg.Done()

		w := NewWriter(fd)
		defer w.Flush()

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
					w.Write(columns)
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
				w.WriteAny(csv_row)

			case <-time.After(5 * time.Second):
				w.Flush()
			}

		}

	}()

	return result, nil
}
