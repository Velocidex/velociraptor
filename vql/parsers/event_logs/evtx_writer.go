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
package event_logs

import (
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
	"io"

	"www.velocidex.com/golang/evtx"
)

const (
	// Offsets into the EVTX file header. See
	// https://github.com/libyal/libevtx/blob/main/documentation/Windows%20XML%20Event%20Log%20(EVTX).asciidoc
	fileHeaderSize          = 0x1000
	fileHeaderUsedSize      = 0x80
	fileHeaderChecksumStart = 0x78

	// The file header stores the number of chunks in 16 bits.
	maxChunksPerFile = 0xffff

	chunkDataChecksumOffset = 0x34

	// An event record starts with "**\0\0", its size, its record
	// id and its timestamp.
	recordMagic      = 0x00002a2a
	recordHeaderSize = 0x18
)

// Writes carved chunks into a new EVTX file so the recovered events
// can be opened with Event Viewer and other EVTX tools. Each chunk
// carries its own string and template tables so chunks from
// different logs can be combined in one file.
type evtxWriter struct {
	fd io.WriterAt

	chunks      uint64
	next_record uint64

	// Windows only reads on while the record numbers in the chunk
	// headers continue from one chunk to the next, but carved
	// chunks come from many logs. The chunk headers are renumbered
	// - the event record ids shown for each event are not changed.
	next_number uint64

	// The same chunk is often found several times (e.g. the live
	// log and old copies in unallocated space). Write it once.
	seen map[[sha256.Size]byte]bool
}

func newEvtxWriter(fd io.WriterAt) (*evtxWriter, error) {
	self := &evtxWriter{
		fd:          fd,
		seen:        make(map[[sha256.Size]byte]bool),
		next_number: 1,
	}

	// A valid header even if no chunks are written.
	return self, self.writeHeader()
}

// Add the chunk to the file. Returns false if the chunk was skipped
// because it was already written or the file is full.
func (self *evtxWriter) WriteChunk(chunk *carvedChunk) (bool, error) {
	key := sha256.Sum256(chunk.Data)
	if self.seen[key] || self.chunks >= maxChunksPerFile {
		return false, nil
	}

	data := repairChunk(chunk)
	if data == nil {
		return false, nil
	}

	first := binary.LittleEndian.Uint64(data[0x08:])
	last := binary.LittleEndian.Uint64(data[0x10:])
	binary.LittleEndian.PutUint64(data[0x08:], self.next_number)
	binary.LittleEndian.PutUint64(data[0x10:], self.next_number+last-first)
	setChunkHeaderChecksum(data)

	offset := int64(fileHeaderSize) + int64(self.chunks)*evtx.EVTX_CHUNK_SIZE
	_, err := self.fd.WriteAt(data, offset)
	if err != nil {
		return false, err
	}

	// When the output file is on the disk being carved, the scan
	// may find the chunks we wrote. Do not write them again.
	self.seen[key] = true
	self.seen[sha256.Sum256(data)] = true
	self.chunks++
	self.next_number += last - first + 1
	if chunk.Header.LastEventRecID >= self.next_record {
		self.next_record = chunk.Header.LastEventRecID + 1
	}

	return true, nil
}

// Windows refuses to open a file if any chunk has a bad checksum. A
// damaged chunk is cut down to its leading intact records and its
// header and data checksum are rewritten to match. Returns a copy of
// the chunk, or nil if no record survives. The caller sets the header
// checksum.
func repairChunk(chunk *carvedChunk) []byte {
	data := append([]byte{}, chunk.Data...)
	if chunk.HeaderChecksumValid && chunk.DataChecksumValid {
		return data
	}

	free_space := int(binary.LittleEndian.Uint32(data[chunkFreeSpaceOffset:]))

	// Records follow each other from the end of the chunk header.
	// Each starts with a signature and its size, and ends with a
	// copy of the size.
	offset := evtx.EVTX_CHUNK_HEADER_SIZE
	last_offset := 0
	var last_id uint64
	count := uint64(0)
	for offset+recordHeaderSize <= free_space {
		if binary.LittleEndian.Uint32(data[offset:]) != recordMagic {
			break
		}
		size := int(binary.LittleEndian.Uint32(data[offset+4:]))
		if size < recordHeaderSize+4 || offset+size > free_space ||
			int(binary.LittleEndian.Uint32(data[offset+size-4:])) != size {
			break
		}
		last_offset = offset
		last_id = binary.LittleEndian.Uint64(data[offset+8:])
		count++
		offset += size
	}

	if count == 0 {
		return nil
	}

	// Drop everything after the last intact record.
	for i := offset; i < len(data); i++ {
		data[i] = 0
	}

	first_number := binary.LittleEndian.Uint64(data[0x08:])
	binary.LittleEndian.PutUint64(data[0x10:], first_number+count-1)
	binary.LittleEndian.PutUint64(data[0x20:], last_id)
	binary.LittleEndian.PutUint32(data[0x2c:], uint32(last_offset))
	binary.LittleEndian.PutUint32(data[chunkFreeSpaceOffset:], uint32(offset))
	binary.LittleEndian.PutUint32(data[chunkDataChecksumOffset:],
		crc32.ChecksumIEEE(data[evtx.EVTX_CHUNK_HEADER_SIZE:offset]))

	return data
}

// The header checksum covers the first 120 bytes and the
// string/template tables, skipping the flags and the checksum.
func setChunkHeaderChecksum(data []byte) {
	crc := crc32.NewIEEE()
	_, _ = crc.Write(data[:chunkHeaderChecksumStart])
	_, _ = crc.Write(data[chunkHeaderChecksumEnd:evtx.EVTX_CHUNK_HEADER_SIZE])
	binary.LittleEndian.PutUint32(data[chunkHeaderChecksumStart+4:], crc.Sum32())
}

func (self *evtxWriter) Chunks() uint64 {
	return self.chunks
}

// Write the final header. Must be called after the last chunk.
func (self *evtxWriter) Close() error {
	return self.writeHeader()
}

func (self *evtxWriter) writeHeader() error {
	header := make([]byte, fileHeaderSize)
	copy(header, evtx.EVTX_HEADER_MAGIC)

	last_chunk := uint64(0)
	if self.chunks > 0 {
		last_chunk = self.chunks - 1
	}

	binary.LittleEndian.PutUint64(header[0x08:], 0) // First chunk number
	binary.LittleEndian.PutUint64(header[0x10:], last_chunk)
	binary.LittleEndian.PutUint64(header[0x18:], self.next_record)
	binary.LittleEndian.PutUint32(header[0x20:], fileHeaderUsedSize)
	binary.LittleEndian.PutUint16(header[0x24:], 2) // Minor version
	binary.LittleEndian.PutUint16(header[0x26:], 3) // Major version
	binary.LittleEndian.PutUint16(header[0x28:], fileHeaderSize)
	binary.LittleEndian.PutUint16(header[0x2a:], uint16(self.chunks))

	// Flags are left clear: the file is not dirty and not full.
	binary.LittleEndian.PutUint32(header[fileHeaderChecksumStart+4:],
		crc32.ChecksumIEEE(header[:fileHeaderChecksumStart]))

	_, err := self.fd.WriteAt(header, 0)
	return err
}
