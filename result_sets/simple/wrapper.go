package simple

import (
	"context"
	"errors"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/result_sets"
)

// A wrapper that allows us to read a range from a larger result set.
type ResultSetReaderWrapper struct {
	result_sets.ResultSetReader

	start_idx, end_idx uint64
	offset             int64
}

func (self *ResultSetReaderWrapper) SeekToRow(start int64) error {
	self.offset = start
	return self.ResultSetReader.SeekToRow(start + int64(self.start_idx))
}

func (self *ResultSetReaderWrapper) TotalRows() int64 {
	return int64(self.end_idx - self.start_idx)
}

func (self *ResultSetReaderWrapper) JSON(ctx context.Context) (<-chan []byte, error) {
	output := make(chan []byte)
	count := self.start_idx + uint64(self.offset)

	go func() {
		defer close(output)

		if self.start_idx == self.end_idx {
			return
		}

		subctx, cancel := context.WithCancel(ctx)
		defer cancel()

		json_chan, err := self.ResultSetReader.JSON(subctx)
		if err != nil {
			return
		}

		for {
			select {
			case <-subctx.Done():
				return

			case row, ok := <-json_chan:
				if !ok {
					return
				}

				select {
				case <-ctx.Done():
					return

				case output <- row:
				}

				count++
				if self.end_idx > 0 && count >= self.end_idx {
					return
				}
			}
		}
	}()

	return output, nil
}

func (self *ResultSetReaderWrapper) Rows(ctx context.Context) <-chan *ordereddict.Dict {
	output := make(chan *ordereddict.Dict)

	// Our initial row relative to the delegate
	count := self.start_idx + uint64(self.offset)
	go func() {
		defer close(output)

		if self.start_idx == self.end_idx {
			return
		}

		subctx, cancel := context.WithCancel(ctx)
		defer cancel()

		row_chan := self.ResultSetReader.Rows(subctx)
		for {
			select {
			case <-subctx.Done():
				return

			case row, ok := <-row_chan:
				if !ok {
					return
				}

				select {
				case <-ctx.Done():
					return

				case output <- row:
				}

				count++
				if self.end_idx > 0 && count >= self.end_idx {
					return
				}
			}
		}
	}()

	return output
}

func WrapReaderForRange(
	reader result_sets.ResultSetReader,
	start_idx, end_idx uint64) (result_sets.ResultSetReader, error) {

	if end_idx < start_idx {
		return nil, errors.New("Invalid range for reader wrapper")
	}

	// No need to wrap it
	if start_idx == 0 && end_idx == 0 {
		return reader, nil
	}

	if end_idx > uint64(reader.TotalRows()) {
		end_idx = uint64(reader.TotalRows())
	}

	err := reader.SeekToRow(int64(start_idx))
	if err != nil {
		return nil, err
	}

	return &ResultSetReaderWrapper{
		ResultSetReader: reader,
		start_idx:       start_idx,
		end_idx:         end_idx,
	}, nil
}
