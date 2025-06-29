package simple

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	vjson "www.velocidex.com/golang/velociraptor/json"
)

type replacement_record struct {
	OriginalIndex int64  `json:"OriginalIndex"`
	Data          []byte `json:"Data"`
}

type indexEntry struct {
	offset, row_count int64
}

// Update a row in the result set.
func (self *ResultSetWriterImpl) Update(
	index uint64, row *ordereddict.Dict) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Flush()

	serialized, err := vjson.MarshalWithOptions(row, self.opts)
	if err != nil {
		return err
	}

	serialized = append(serialized, '\n')

	err = update_row(self.file_store_factory,
		self.log_path, int64(index), serialized)

	return err
}

// Break a combined JSON blob into line indexes and update the row
// with new data.
func update_row(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec, idx int64,
	serialized []byte) error {
	idx_writer, err := file_store_factory.WriteFile(log_path.
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
	if err != nil {
		return err
	}
	defer idx_writer.Close()

	fd_writer, err := file_store_factory.WriteFile(log_path)
	if err != nil {
		return err
	}
	defer fd_writer.Close()

	fd, err := file_store_factory.ReadFile(log_path)
	if err != nil {
		return err
	}
	defer fd.Close()

	idx_fd, err := file_store_factory.ReadFile(log_path.
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
	if err != nil {
		return err
	}
	defer idx_fd.Close()

	// Read the index at the required offset
	// Get the index entry for this row
	_, err = idx_fd.Seek(8*idx, io.SeekStart)
	if err != nil {
		return err
	}

	value := int64(0)
	err = binary.Read(idx_fd, binary.LittleEndian, &value)
	if err != nil {
		return err
	}

	// The value contains the file offset and the row count.
	offset := value & offset_mask
	row_count := value >> 40

	// The row refers to an index block - the index contains a list of
	// offset, row_count starting from the start of the block. We can
	// find the start of the block by subtracting the row_count from
	// the offset of this index entry. For example a single JSON blob
	// of 5 rows has the following index entries:

	// offset: 500, row_count: 0
	// offset: 500, row_count: 1
	// offset: 500, row_count: 2  <--- Seeking into this entry, the start
	//                                 of the index block is 2 entries back
	// offset: 500, row_count: 3
	// offset: 500, row_count: 4

	// The index into the start of the block.
	block_idx := idx - row_count
	block_offset := offset

	_, err = idx_fd.Seek(8*block_idx, io.SeekStart)
	if err != nil {
		return err
	}

	var index_block []indexEntry
	for i := int64(0); ; i++ {
		err = binary.Read(idx_fd, binary.LittleEndian, &value)
		if err != nil {
			break
		}

		offset := value & offset_mask
		row_count := value >> 40

		if offset != block_offset || row_count != i {
			break
		}

		index_block = append(index_block, indexEntry{
			offset:    offset,
			row_count: row_count,
		})
	}

	// Update the index by reading the json blob and splitting it on
	// new lines.
	_, err = fd.Seek(offset, io.SeekStart)
	if err != nil {
		return err
	}

	offset_buffer := new(bytes.Buffer)

	// Consume rows from the start of the blob to reach our
	// desired row count.
	reader := bufio.NewReader(fd)
	for i := 0; i < len(index_block); i++ {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}

		// We need to replace this line.
		if block_idx+int64(i) == idx {
			// There are two cases, the new entry is smaller than the
			// old entry - just overwrite.
			if len(line) >= len(serialized) {
				// Pad the serialized string with spaces so it takes
				// the same length.
				for i := len(serialized); i < len(line); i++ {
					serialized = append(serialized, ' ')
				}

				err := fd_writer.Update(serialized, block_offset)
				if err != nil {
					return err
				}

				// Otherwise write a pointer record to the end of the
				// file.
			} else {
				replacement := &replacement_record{
					Data:          serialized,
					OriginalIndex: idx,
				}

				replacement_bytes, err := json.Marshal(replacement)
				if err != nil {
					return err
				}

				// Write the data at the end of the file.
				end_offset, err := fd_writer.Size()
				if err != nil {
					return err
				}

				replacement_bytes = append([]byte{'@'}, replacement_bytes...)
				replacement_bytes = append(replacement_bytes, '\n')

				_, err = fd_writer.Write(replacement_bytes)
				if err != nil {
					return err
				}

				// Write the reference in place of the original data.
				serialized_ptr := []byte(fmt.Sprintf("@%d\n", end_offset))
				for i := len(serialized_ptr); i < len(line); i++ {
					serialized_ptr = append(serialized_ptr, ' ')
				}

				err = fd_writer.Update(serialized_ptr, block_offset)
				if err != nil {
					return err
				}
			}
		}

		// Write the new index entry.
		err = binary.Write(offset_buffer, binary.LittleEndian, block_offset)
		if err != nil {
			return err
		}
		block_offset += int64(len(line))
	}

	err = idx_writer.Update(offset_buffer.Bytes(), 8*block_idx)
	if err != nil {
		return err
	}
	return nil
}
