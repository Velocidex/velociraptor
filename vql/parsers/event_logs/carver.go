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
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"time"

	"github.com/dustin/go-humanize"
	"www.velocidex.com/golang/evtx"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/utils"
	vfilter "www.velocidex.com/golang/vfilter"
)

const (
	// How much data we scan for the chunk signature in each read.
	carveBlockSize = 1024 * 1024

	// Offsets into the chunk header. See
	// https://github.com/libyal/libevtx/blob/main/documentation/Windows%20XML%20Event%20Log%20(EVTX).asciidoc
	chunkHeaderSizeValue     = 0x80
	chunkFreeSpaceOffset     = 0x30
	chunkHeaderChecksumStart = 0x78
	chunkHeaderChecksumEnd   = 0x80

	// The library refuses to parse more records than this from a
	// single chunk so a header claiming more is not plausible.
	maxRecordsPerChunk = 1024 * 10

	// LZNT1 block header: bit 15 is set for compressed blocks,
	// bits 12-14 are always 3 and the low 12 bits are the size of
	// the block minus 3.
	lznt1BlockSize            = 0x1000
	lznt1UnitSize             = 0x10000
	chunkOffsetInUnit         = 0x1000
	lznt1PrefixSize           = 3
	lznt1SignatureMask        = 0x7000
	lznt1Signature            = 0x3000
	lznt1CompressedHeaderMask = 0xb000
	lznt1SizeMask             = 0x0fff
)

var chunkMagic = []byte(evtx.EVTX_CHUNK_HEADER_MAGIC)

// A chunk signature found in the carved data.
type carvedChunk struct {
	// Offset of the chunk in the carved stream.
	Offset int64

	Header evtx.ChunkHeader

	// The CRC32 of the chunk header matches the stored checksum.
	HeaderChecksumValid bool

	// The CRC32 of the event records matches the stored checksum.
	DataChecksumValid bool

	// The stream ended before the full chunk could be read. The
	// missing data is zero filled.
	Truncated bool

	// The chunk was found inside an NTFS compressed file and was
	// decompressed.
	Compressed bool

	// The full chunk, always EVTX_CHUNK_SIZE long.
	Data []byte
}

type carveStats struct {
	BytesScanned int64
	Candidates   int64
	Chunks       int64

	// Why the scan stopped before reaching the end offset, if it did.
	StopError error
}

// Scan the reader between start and end (end <= 0 means until the
// end of the stream) for EVTX chunks. Signatures are searched at any
// byte offset because chunks in memory, the pagefile or unallocated
// space are not necessarily aligned. Only chunks with a plausible
// header are emitted - their checksums are reported but not enforced
// because partially overwritten chunks still contain useful records.
func carveChunks(
	ctx context.Context,
	scope vfilter.Scope,
	reader io.ReaderAt,
	start, end int64, stats *carveStats) <-chan *carvedChunk {

	output_chan := make(chan *carvedChunk)

	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		logger, closer := utils.NewDeduplicatedLogger(30 * time.Second)
		defer closer()

		start_time := utils.GetTime().Now()

		buf := make([]byte, carveBlockSize)

		// The end of the previous block is kept so a signature
		// straddling two blocks is still found. Carrying it in
		// memory keeps every read at the block offsets.
		overlap := len(chunkMagic) - 1
		window := make([]byte, 0, overlap+carveBlockSize)
		var tail []byte

		// Scan from an aligned offset and ignore signatures before
		// the start.
		offset := start
		for end <= 0 || offset < end {
			to_read := int64(len(buf))
			if end > 0 && offset+to_read > end {
				to_read = end - offset
			}

			// Readers may return fewer bytes than requested, with or
			// without an error. Only a read returning nothing ends
			// the scan.
			n, err := reader.ReadAt(buf[:to_read], int64(offset))
			if n <= 0 {
				if err != nil && !errors.Is(err, io.EOF) {
					stats.StopError = err
				}
				return
			}

			window = append(append(window[:0], tail...), buf[:n]...)
			window_offset := offset - int64(len(tail))

			for idx := 0; idx < len(window); {
				hit := bytes.Index(window[idx:], chunkMagic)
				if hit < 0 {
					break
				}

				chunk_offset := window_offset + int64(idx+hit)
				idx += hit + 1
				if chunk_offset < start {
					continue
				}

				stats.Candidates++
				chunk, ok := readChunk(reader, chunk_offset)
				if !ok {
					continue
				}
				stats.Chunks++

				select {
				case <-ctx.Done():
					return
				case output_chan <- chunk:
				}
			}

			// A full signature can never be inside the tail, so no
			// signature is counted twice.
			tail_start := len(window) - overlap
			if tail_start < 0 {
				tail_start = 0
			}
			tail = append(tail[:0], window[tail_start:]...)

			offset += int64(n)

			stats.BytesScanned = offset - start

			// Report progress - this could take a while so it is nice
			// to know how it's going.
			logger.Log(
				// This will be called a lot so log lazily.
				func(msg string, args ...interface{}) {
					scope.Log("carve_evtx: scanned %v bytes in %v",
						humanize.Bytes(uint64(stats.BytesScanned)),
						utils.GetTime().Now().Sub(
							start_time).Round(time.Second))
				}, "")

			if errors.Is(err, io.EOF) {
				return
			}

			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	return output_chan
}

// Read the chunk at offset and check that its header is plausible.
func readChunk(reader io.ReaderAt, offset int64) (*carvedChunk, bool) {
	data := make([]byte, evtx.EVTX_CHUNK_SIZE)
	n, _ := reader.ReadAt(data, offset)
	result, ok := checkChunk(offset, data, n)
	if ok {
		return result, true
	}

	data, n = readCompressedChunk(reader, offset)
	result, ok = checkChunk(offset, data, n)
	if ok {
		result.Compressed = true
	}
	return result, ok
}

// NTFS compresses files in units of 16 clusters using LZNT1, and
// the event logs are compressed by default. Every 4kb block in a
// unit is compressed separately, so a chunk starting on a block
// boundary still has its signature as 8 literal bytes - preceded by
// the 2 byte block header and a zero flag byte.
//
// Chunks follow the 4kb file header so a chunk spans two
// compression units. Each unit starts on a new cluster which we
// assume to be 4kb.
func readCompressedChunk(reader io.ReaderAt, offset int64) ([]byte, int) {
	start := offset - lznt1PrefixSize
	if start < 0 {
		return nil, 0
	}

	// Compressed data is never larger than the uncompressed data
	// plus block headers and the slack at the end of a unit.
	raw := make([]byte, evtx.EVTX_CHUNK_SIZE+2*lznt1BlockSize)
	n, _ := reader.ReadAt(raw, start)
	raw = raw[:n]

	if n < lznt1PrefixSize || raw[2] != 0 ||
		binary.LittleEndian.Uint16(raw)&lznt1CompressedHeaderMask !=
			lznt1CompressedHeaderMask {
		return nil, 0
	}

	data := make([]byte, 0, evtx.EVTX_CHUNK_SIZE)
	skipped_to_next_unit := false
	i := 0
	for len(data) < evtx.EVTX_CHUNK_SIZE && i+2 <= len(raw) {
		header := binary.LittleEndian.Uint16(raw[i:])

		// The compression unit has ended - either we have read all
		// its blocks or there is no block header. The next unit
		// starts on the next cluster.
		unit_full := len(data) == lznt1UnitSize-chunkOffsetInUnit
		if (unit_full && !skipped_to_next_unit) ||
			header&lznt1SignatureMask != lznt1Signature {
			if skipped_to_next_unit {
				// The next unit was not compressible so it is
				// stored as is.
				remaining := evtx.EVTX_CHUNK_SIZE - len(data)
				if remaining > len(raw)-i {
					remaining = len(raw) - i
				}
				data = append(data, raw[i:i+remaining]...)
				break
			}
			skipped_to_next_unit = true

			next_unit := start + int64(i) + lznt1BlockSize - 1
			next_unit -= next_unit % lznt1BlockSize
			i = int(next_unit - start)
			continue
		}

		block_end := i + int(header&lznt1SizeMask) + 3
		if block_end > len(raw) {
			break
		}

		block, err := lznt1Decompress(raw[i:block_end])
		if err != nil || len(block) == 0 || len(block) > lznt1BlockSize {
			break
		}
		data = append(data, block...)
		i = block_end
	}

	if len(data) > evtx.EVTX_CHUNK_SIZE {
		data = data[:evtx.EVTX_CHUNK_SIZE]
	}
	n = len(data)

	// Always return a full chunk, zero filled.
	return append(data, make([]byte, evtx.EVTX_CHUNK_SIZE-n)...), n
}

// Check that the chunk has a plausible header. n is the number of
// valid bytes in data.
func checkChunk(offset int64, data []byte, n int) (*carvedChunk, bool) {
	if n < evtx.EVTX_CHUNK_HEADER_SIZE {
		return nil, false
	}

	result := &carvedChunk{
		Offset:    offset,
		Data:      data,
		Truncated: n < evtx.EVTX_CHUNK_SIZE,
	}

	err := binary.Read(bytes.NewReader(data),
		binary.LittleEndian, &result.Header)
	if err != nil {
		return nil, false
	}

	header := &result.Header
	free_space := binary.LittleEndian.Uint32(data[chunkFreeSpaceOffset:])

	if header.HeaderSize != chunkHeaderSizeValue ||
		header.FirstEventRecNumber > header.LastEventRecNumber ||
		header.LastEventRecNumber-header.FirstEventRecNumber >= maxRecordsPerChunk ||
		header.LastEventRecOffset >= evtx.EVTX_CHUNK_SIZE ||
		free_space < evtx.EVTX_CHUNK_HEADER_SIZE ||
		free_space > evtx.EVTX_CHUNK_SIZE {
		return nil, false
	}

	// The header checksum covers the first 120 bytes and the
	// string/template tables, skipping the flags and the checksum.
	crc := crc32.NewIEEE()
	_, _ = crc.Write(data[:chunkHeaderChecksumStart])
	_, _ = crc.Write(data[chunkHeaderChecksumEnd:evtx.EVTX_CHUNK_HEADER_SIZE])
	result.HeaderChecksumValid = crc.Sum32() == header.CheckSum

	result.DataChecksumValid = crc32.ChecksumIEEE(
		data[evtx.EVTX_CHUNK_HEADER_SIZE:free_space]) == header.EventRecordCheckSum

	return result, true
}

// LZNT1Decompress can panic on malformed blocks (e.g. a back
// reference truncated by the end of the block). Carved data is often
// malformed so contain any panic to this block.
func lznt1Decompress(block []byte) (result []byte, err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("LZNT1 decompression panic: %v", r)
			utils.PrintStack()
		}
	}()

	return ntfs.LZNT1Decompress(block)
}

// Parse the records in a carved chunk. Carved data may be corrupt so
// any panic in the parser is contained to this chunk.
func parseCarvedChunk(chunk *carvedChunk) (records []*evtx.EventRecord, err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = errors.New("parser panic while parsing carved chunk")
			utils.PrintStack()
		}
	}()

	parsed, err := evtx.NewChunk(bytes.NewReader(chunk.Data), 0)
	if err != nil {
		return nil, err
	}

	return parsed.Parse(0)
}
