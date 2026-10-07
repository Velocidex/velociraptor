package event_logs

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/evtx"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// This log contains a single chunk recording the System log being
// cleared (Event ID 104).
const testEvtx = "../../../artifacts/testdata/files/DE_104_system_log_cleared.evtx"

func getTestChunk(t *testing.T) []byte {
	data, err := os.ReadFile(testEvtx)
	assert.NoError(t, err)

	// The first chunk follows the 4kb file header.
	chunk := data[0x1000 : 0x1000+evtx.EVTX_CHUNK_SIZE]
	assert.Equal(t, evtx.EVTX_CHUNK_HEADER_MAGIC, string(chunk[:8]))
	return chunk
}

func noise(rng *rand.Rand, size int) []byte {
	result := make([]byte, size)
	rng.Read(result)
	return result
}

func carveAll(t *testing.T, image []byte, start, end int64) (
	[]*carvedChunk, *carveStats) {
	scope := vql_subsystem.MakeScope()

	stats := &carveStats{}
	var result []*carvedChunk
	for chunk := range carveChunks(context.Background(), scope,
		bytes.NewReader(image), start, end, stats) {
		result = append(result, chunk)
	}
	return result, stats
}

func TestCarveChunks(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	chunk := getTestChunk(t)

	// A copy of the chunk with a damaged string table - the header
	// is still plausible but its checksum no longer matches.
	damaged := append([]byte{}, chunk...)
	damaged[0x100] ^= 0xff

	image := &bytes.Buffer{}
	image.Write(noise(rng, 100003))

	// 1. An intact chunk at an unaligned offset.
	intact_offset := int64(image.Len())
	image.Write(chunk)
	image.Write(noise(rng, 5000))

	// A false positive: the signature followed by garbage.
	image.WriteString(evtx.EVTX_CHUNK_HEADER_MAGIC)
	image.Write(noise(rng, 1000))

	// 2. A chunk with a damaged header.
	damaged_offset := int64(image.Len())
	image.Write(damaged)
	image.Write(noise(rng, 7777))

	// 3. A chunk cut off by the end of the image.
	truncated_offset := int64(image.Len())
	image.Write(chunk[:0x8000])

	chunks, stats := carveAll(t, image.Bytes(), 0, 0)

	assert.Equal(t, int64(image.Len()), stats.BytesScanned)
	assert.Equal(t, int64(4), stats.Candidates)
	assert.Equal(t, int64(3), stats.Chunks)
	assert.Equal(t, 3, len(chunks))

	assert.Equal(t, intact_offset, chunks[0].Offset)
	assert.True(t, chunks[0].HeaderChecksumValid)
	assert.True(t, chunks[0].DataChecksumValid)
	assert.False(t, chunks[0].Truncated)

	assert.Equal(t, damaged_offset, chunks[1].Offset)
	assert.False(t, chunks[1].HeaderChecksumValid)
	assert.True(t, chunks[1].DataChecksumValid)

	assert.Equal(t, truncated_offset, chunks[2].Offset)
	assert.True(t, chunks[2].HeaderChecksumValid)
	assert.True(t, chunks[2].Truncated)
	assert.Equal(t, evtx.EVTX_CHUNK_SIZE, len(chunks[2].Data))

	// All the records in the intact chunk are recovered.
	records, err := parseCarvedChunk(chunks[0])
	assert.NoError(t, err)
	header := chunks[0].Header
	assert.Equal(t, int(header.LastEventRecNumber-header.FirstEventRecNumber+1),
		len(records))

	// The log cleared event is among them.
	found := false
	for _, record := range records {
		row := makeCarvedRow(chunks[0], record, nil)
		assert.NotNil(t, row)

		value, _ := ordereddict.GetAny(row, "System.EventID.Value")
		event_id, _ := utils.ToInt64(value)
		if event_id == 104 {
			found = true
		}

		chunk_info_any, pres := row.Get("ChunkInfo")
		assert.True(t, pres)

		chunk_offset, _ := chunk_info_any.(*ordereddict.Dict).GetInt64("Offset")
		assert.Equal(t, intact_offset, chunk_offset)
	}
	assert.True(t, found)
}

// A signature split across two read blocks must be found exactly once.
func TestCarveChunkAcrossBlockBoundary(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	chunk := getTestChunk(t)

	for _, delta := range []int{-8, -7, -3, -1, 0, 1} {
		offset := carveBlockSize + delta
		image := noise(rng, offset)
		image = append(image, chunk...)
		image = append(image, noise(rng, 100)...)

		chunks, _ := carveAll(t, image, 0, 0)
		assert.Equal(t, 1, len(chunks), "delta %v", delta)
		assert.Equal(t, int64(offset), chunks[0].Offset, "delta %v", delta)
	}
}

func TestCarveRange(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	chunk := getTestChunk(t)

	image := noise(rng, 1000)
	image = append(image, chunk...)
	second := len(image)
	image = append(image, chunk...)

	// Skipping past the first chunk only finds the second.
	chunks, _ := carveAll(t, image, 1001, 0)
	assert.Equal(t, 1, len(chunks))
	assert.Equal(t, int64(second), chunks[0].Offset)

	// A range ending before the second chunk only finds the first.
	chunks, _ = carveAll(t, image, 0, int64(second))
	assert.Equal(t, 1, len(chunks))
	assert.Equal(t, int64(1000), chunks[0].Offset)
}

// Accessors return a ReadSeekReaderAdapter rather than a plain
// io.ReaderAt. Its EOF flag is sticky, so a chunk read that runs past
// the end of the stream must not stop the scan early.
func TestCarveThroughAccessorAdapter(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	chunk := getTestChunk(t)

	image := noise(rng, carveBlockSize/2)
	first := len(image)
	image = append(image, chunk...)
	image = append(image, noise(rng, 2*carveBlockSize)...)
	second := len(image)
	image = append(image, chunk...)
	image = append(image, noise(rng, 1000)...)

	// A chunk cut off by the end of the stream.
	third := len(image)
	image = append(image, chunk[:0x4000]...)

	scope := vql_subsystem.MakeScope()

	for _, with_size := range []bool{false, true} {
		adapter := utils.NewReadSeekReaderAdapter(bytes.NewReader(image), nil)
		if with_size {
			adapter.SetSize(int64(len(image)))
		}

		stats := &carveStats{}
		var offsets []int64
		for c := range carveChunks(context.Background(), scope,
			utils.MakeReaderAtter(adapter), 0, 0, stats) {
			offsets = append(offsets, c.Offset)
		}

		assert.Equal(t, []int64{int64(first), int64(second), int64(third)},
			offsets, "with_size %v", with_size)
		assert.Equal(t, int64(len(image)), stats.BytesScanned,
			"with_size %v", with_size)
	}
}

// Behaves like a raw Windows device which rejects reads that are not
// sector aligned.
type deviceReader struct {
	reader *bytes.Reader
}

func (self *deviceReader) ReadAt(buf []byte, offset int64) (int, error) {
	if offset%512 != 0 || len(buf)%512 != 0 {
		return 0, errors.New("The parameter is incorrect.")
	}
	return self.reader.ReadAt(buf, offset)
}

func NewDeviceReader(data []byte) (io.ReaderAt, error) {
	return utils.NewPagedReader(
		&deviceReader{bytes.NewReader(data)},
		4096, 100)
}

// Unaligned chunks and offsets must still be carved from a device.
func TestCarveFromAlignedDevice(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	chunk := getTestChunk(t)

	image := noise(rng, 3001)
	unaligned := len(image)
	image = append(image, chunk...)
	image = append(image, noise(rng, 4096-(len(image)%4096))...)
	aligned := len(image)
	image = append(image, chunk...)
	image = append(image, noise(rng, carveBlockSize+123)...)
	last := len(image)
	image = append(image, chunk[:0x3000]...)

	scope := vql_subsystem.MakeScope()

	for _, tc := range []struct {
		start, end int64
		expected   []int64
	}{
		{0, 0, []int64{int64(unaligned), int64(aligned), int64(last)}},
		{int64(unaligned), 0, []int64{int64(unaligned), int64(aligned), int64(last)}},
		{int64(unaligned) + 1, int64(last) + 777,
			[]int64{int64(aligned), int64(last)}},
		{0, int64(aligned) + 5, []int64{int64(unaligned)}},
	} {
		stats := &carveStats{}
		var offsets []int64
		var chunks []*carvedChunk
		reader, err := NewDeviceReader(image)
		assert.NoError(t, err)

		for c := range carveChunks(context.Background(), scope,
			reader,
			tc.start, tc.end, stats) {
			offsets = append(offsets, c.Offset)
			chunks = append(chunks, c)
		}

		assert.NoError(t, stats.StopError, "range %v-%v", tc.start, tc.end)
		assert.Equal(t, tc.expected, offsets, "range %v-%v", tc.start, tc.end)

		for _, c := range chunks {
			assert.True(t, c.HeaderChecksumValid, "chunk %v", c.Offset)
			assert.Equal(t, c.Offset == int64(last), c.Truncated, "chunk %v", c.Offset)
			assert.Equal(t, chunk[:0x3000], c.Data[:0x3000], "chunk %v", c.Offset)
		}
	}
}

func TestAlignedReadAt(t *testing.T) {
	data := noise(rand.New(rand.NewSource(7)), 10000)
	reader, err := NewDeviceReader(data)
	assert.NoError(t, err)

	for _, tc := range []struct {
		offset int64
		length int
		n      int
		eof    bool
	}{
		{0, 4096, 4096, false},
		{1, 10, 10, false},
		{4095, 2, 2, false},
		{9990, 10, 10, false},
		{9990, 20, 10, true},
		{10000, 10, 0, true},
		{20000, 10, 0, true},
	} {
		buf := make([]byte, tc.length)
		n, err := reader.ReadAt(buf, tc.offset)
		assert.Equal(t, tc.n, n, "offset %v", tc.offset)
		assert.Equal(t, tc.eof, errors.Is(err, io.EOF), "offset %v err %v", tc.offset, err)
		if n > 0 {
			assert.Equal(t, data[tc.offset:tc.offset+int64(n)], buf[:n], "offset %v", tc.offset)
		}
	}
}

// Compress a block of up to 4kb with LZNT1 the way NTFS does.
func lznt1CompressBlock(in []byte) []byte {
	out := []byte{0, 0}
	pos := 0
	for pos < len(in) {
		flag_idx := len(out)
		out = append(out, 0)

		for bit := 0; bit < 8 && pos < len(in); bit++ {
			// The split between offset and length bits depends on
			// the position in the block.
			length_bits := 12
			for p := pos - 1; p >= 0x10; p >>= 1 {
				length_bits--
			}
			max_offset := 1 << (16 - length_bits)
			max_length := (1 << length_bits) - 1 + 3

			best_len, best_off := 0, 0
			for off := 1; off <= pos && off <= max_offset; off++ {
				l := 0
				for l < max_length && pos+l < len(in) && in[pos+l-off] == in[pos+l] {
					l++
				}
				if l > best_len {
					best_len, best_off = l, off
				}
				if l == max_length {
					break
				}
			}

			if best_len >= 3 {
				token := uint16(best_off-1)<<length_bits | uint16(best_len-3)
				out = binary.LittleEndian.AppendUint16(out, token)
				out[flag_idx] |= 1 << bit
				pos += best_len
			} else {
				out = append(out, in[pos])
				pos++
			}
		}
	}

	// Store incompressible blocks as is.
	if len(out)-2 >= lznt1BlockSize {
		out = binary.LittleEndian.AppendUint16(nil, 0x3000|uint16(len(in)-1))
		return append(out, in...)
	}
	binary.LittleEndian.PutUint16(out, 0xb000|uint16(len(out)-3))
	return out
}

// Lay out a file on disk the way NTFS compression does: 64kb units
// each starting on a new 4kb cluster. The slack at the end of a unit
// holds stale data. Units listed in raw_units are stored
// uncompressed.
func ntfsCompress(rng *rand.Rand, file []byte, raw_units ...int) []byte {
	var result []byte
	for unit_idx := 0; unit_idx*lznt1UnitSize < len(file); unit_idx++ {
		unit := file[unit_idx*lznt1UnitSize:]
		if len(unit) > lznt1UnitSize {
			unit = unit[:lznt1UnitSize]
		}

		var compressed []byte
		for i := 0; i < len(unit); i += lznt1BlockSize {
			end := i + lznt1BlockSize
			if end > len(unit) {
				end = len(unit)
			}
			compressed = append(compressed, lznt1CompressBlock(unit[i:end])...)
		}

		for _, raw := range raw_units {
			if raw == unit_idx {
				compressed = unit
			}
		}

		result = append(result, compressed...)
		if rem := len(result) % lznt1BlockSize; rem != 0 {
			result = append(result, noise(rng, lznt1BlockSize-rem)...)
		}
	}
	return result
}

func TestLZNT1Compressor(t *testing.T) {
	data, err := os.ReadFile(testEvtx)
	assert.NoError(t, err)

	for i := 0; i < len(data); i += lznt1BlockSize {
		block := data[i : i+lznt1BlockSize]
		decompressed, err := ntfs.LZNT1Decompress(lznt1CompressBlock(block))
		assert.NoError(t, err)
		assert.Equal(t, block, decompressed, "block at %#x", i)
	}
}

// The event logs are NTFS compressed by default so carving a raw
// volume needs to decompress chunks.
func TestCarveCompressedChunk(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	file, err := os.ReadFile(testEvtx)
	assert.NoError(t, err)
	chunk := getTestChunk(t)

	for _, raw_units := range [][]int{nil, {1}} {
		disk := noise(rng, 7*lznt1BlockSize)
		disk = append(disk, ntfsCompress(rng, file, raw_units...)...)
		disk = append(disk, noise(rng, 3*lznt1BlockSize)...)

		// The plain chunk header is not visible on disk.
		assert.False(t, bytes.Contains(disk, chunk[:evtx.EVTX_CHUNK_HEADER_SIZE]))

		chunks, stats := carveAll(t, disk, 0, 0)
		assert.Equal(t, int64(1), stats.Chunks, "raw units %v", raw_units)
		if len(chunks) != 1 {
			continue
		}

		c := chunks[0]
		assert.True(t, c.Compressed)
		assert.True(t, c.HeaderChecksumValid)
		assert.True(t, c.DataChecksumValid)
		assert.False(t, c.Truncated)
		assert.Equal(t, chunk, c.Data, "raw units %v", raw_units)

		records, err := parseCarvedChunk(c)
		assert.NoError(t, err)
		found := false
		for _, record := range records {
			row := makeCarvedRow(c, record, nil)
			value, _ := ordereddict.GetAny(row, "System.EventID.Value")
			event_id, _ := utils.ToInt64(value)
			found = found || event_id == 104

			chunk_info_any, pres := row.Get("ChunkInfo")
			assert.True(t, pres)
			compressed, _ := chunk_info_any.(*ordereddict.Dict).Get("Compressed")
			assert.Equal(t, true, compressed)
		}
		assert.True(t, found)
	}
}

// A compressed chunk cut off by the end of the image is still
// recovered as far as possible.
func TestCarveTruncatedCompressedChunk(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	file, err := os.ReadFile(testEvtx)
	assert.NoError(t, err)

	disk := noise(rng, lznt1BlockSize)
	compressed := ntfsCompress(rng, file)
	disk = append(disk, compressed[:len(compressed)/3]...)

	chunks, _ := carveAll(t, disk, 0, 0)
	assert.Equal(t, 1, len(chunks))
	if len(chunks) == 1 {
		assert.True(t, chunks[0].Compressed)
		assert.True(t, chunks[0].Truncated)
		assert.True(t, chunks[0].HeaderChecksumValid)
	}
}

// A compressed block whose last token is a back reference cut short
// by the end of the block. This crashed LZNT1Decompress on a real
// volume.
func truncatedReferenceBlock() []byte {
	block := []byte{0x0a, 0xb0, 0x00}
	block = append(block, evtx.EVTX_CHUNK_HEADER_MAGIC...)
	return append(block, 0x01, 0x41)
}

func TestCarveTruncatedLZNT1Reference(t *testing.T) {
	_, err := lznt1Decompress(truncatedReferenceBlock())
	assert.Error(t, err)

	rng := rand.New(rand.NewSource(10))
	disk := noise(rng, 5000)
	disk = append(disk, truncatedReferenceBlock()...)
	disk = append(disk, noise(rng, 5000)...)

	chunks, stats := carveAll(t, disk, 0, 0)
	assert.Equal(t, 0, len(chunks))
	assert.Equal(t, int64(1), stats.Candidates)
}

// Carved data is attacker controlled - run with
// go test -fuzz FuzzCarveChunk ./vql/parsers/event_logs/
func FuzzCarveChunk(f *testing.F) {
	scope := vql_subsystem.MakeScope()

	data, err := os.ReadFile(testEvtx)
	if err == nil && len(data) >= 0x1000+evtx.EVTX_CHUNK_SIZE {
		f.Add(data[0x1000 : 0x1000+evtx.EVTX_CHUNK_SIZE])
	}
	f.Add([]byte(evtx.EVTX_CHUNK_HEADER_MAGIC))
	f.Add(truncatedReferenceBlock())
	if err == nil {
		f.Add(ntfsCompress(rand.New(rand.NewSource(1)), data))
	}

	f.Fuzz(func(t *testing.T, image []byte) {
		stats := &carveStats{}
		for c := range carveChunks(context.Background(), scope,
			bytes.NewReader(image), 0, 0, stats) {
			records, _ := parseCarvedChunk(c)
			for _, r := range records {
				makeCarvedRow(c, r, nil)
			}
		}
	})
}

// Random data with forced signatures must never crash the carver.
func TestCarveGarbage(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	chunk := getTestChunk(t)

	for i := 0; i < 50; i++ {
		// Keep a valid looking header but scramble the records.
		data := append([]byte{}, chunk...)
		rng.Read(data[evtx.EVTX_CHUNK_HEADER_SIZE:])

		chunks, _ := carveAll(t, data, 0, 0)
		for _, c := range chunks {
			records, _ := parseCarvedChunk(c)
			for _, r := range records {
				makeCarvedRow(c, r, nil)
			}
		}
	}
}
