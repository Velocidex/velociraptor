package event_logs

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"testing"

	"www.velocidex.com/golang/evtx"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// An in memory io.WriterAt.
type memWriter struct {
	buf []byte
}

func (self *memWriter) WriteAt(p []byte, off int64) (int, error) {
	if end := int(off) + len(p); end > len(self.buf) {
		self.buf = append(self.buf, make([]byte, end-len(self.buf))...)
	}
	return copy(self.buf[off:], p), nil
}

func mustCheckChunk(t *testing.T, data []byte) *carvedChunk {
	chunk, ok := checkChunk(0, data, len(data))
	assert.True(t, ok)
	return chunk
}

// Chunks of the second log in the testdata.
func otherLogChunks(t *testing.T) []*carvedChunk {
	data, err := os.ReadFile("../../../artifacts/testdata/files/RDPAuth_Security.evtx")
	assert.NoError(t, err)

	var result []*carvedChunk
	for off := 0x1000; off+evtx.EVTX_CHUNK_SIZE <= len(data); off += evtx.EVTX_CHUNK_SIZE {
		chunk, ok := checkChunk(int64(off), data[off:off+evtx.EVTX_CHUNK_SIZE],
			evtx.EVTX_CHUNK_SIZE)
		if ok {
			result = append(result, chunk)
		}
	}
	assert.True(t, len(result) > 2)
	return result
}

func recordIds(t *testing.T, chunk *carvedChunk) []uint64 {
	records, err := parseCarvedChunk(chunk)
	assert.NoError(t, err)
	var result []uint64
	for _, r := range records {
		result = append(result, r.Header.RecordID)
	}
	return result
}

func TestEvtxWriter(t *testing.T) {
	intact := mustCheckChunk(t, getTestChunk(t))
	others := otherLogChunks(t)

	// The records after the middle of the used space are overwritten.
	damaged_data := append([]byte{}, others[1].Data...)
	free_space := int(binary.LittleEndian.Uint32(damaged_data[chunkFreeSpaceOffset:]))
	mid := evtx.EVTX_CHUNK_HEADER_SIZE + (free_space-evtx.EVTX_CHUNK_HEADER_SIZE)/2
	copy(damaged_data[mid:free_space], noise(newRng(), free_space-mid))
	damaged := mustCheckChunk(t, damaged_data)
	assert.False(t, damaged.DataChecksumValid)

	// Nothing survives when the first record is destroyed.
	hopeless_data := append([]byte{}, others[2].Data...)
	copy(hopeless_data[evtx.EVTX_CHUNK_HEADER_SIZE:], "XXXX")
	hopeless := mustCheckChunk(t, hopeless_data)

	out := &memWriter{}
	writer, err := newEvtxWriter(out)
	assert.NoError(t, err)

	// Chunks from different logs, a duplicate and damaged chunks.
	for _, c := range []*carvedChunk{others[0], intact, others[0], damaged, hopeless} {
		_, err := writer.WriteChunk(c)
		assert.NoError(t, err)
	}
	assert.NoError(t, writer.Close())
	assert.Equal(t, uint64(3), writer.Chunks())

	// The library reads the file back.
	chunks, err := evtx.GetChunks(bytes.NewReader(out.buf))
	assert.NoError(t, err)
	assert.Equal(t, 3, len(chunks))

	// Every written chunk has valid checksums and the record
	// numbers in the chunk headers follow on from each other.
	next := uint64(1)
	var written []*carvedChunk
	for i := 0; i < 3; i++ {
		off := fileHeaderSize + i*evtx.EVTX_CHUNK_SIZE
		c := mustCheckChunk(t, out.buf[off:off+evtx.EVTX_CHUNK_SIZE])
		assert.True(t, c.HeaderChecksumValid, "chunk %v", i)
		assert.True(t, c.DataChecksumValid, "chunk %v", i)
		assert.Equal(t, next, c.Header.FirstEventRecNumber, "chunk %v", i)
		next = c.Header.LastEventRecNumber + 1
		written = append(written, c)
	}

	// Event record ids are kept as they were.
	assert.Equal(t, recordIds(t, others[0]), recordIds(t, written[0]))
	assert.Equal(t, recordIds(t, intact), recordIds(t, written[1]))

	// Only the intact leading records of the damaged chunk are kept.
	repaired := recordIds(t, written[2])
	original := recordIds(t, others[1])
	assert.True(t, len(repaired) > 0)
	assert.True(t, len(repaired) < len(original))
	assert.Equal(t, original[:len(repaired)], repaired)

	// The file header describes the chunks.
	header := out.buf[:fileHeaderSize]
	assert.Equal(t, evtx.EVTX_HEADER_MAGIC, string(header[:8]))
	assert.Equal(t, uint16(3), binary.LittleEndian.Uint16(header[0x2a:]))
	assert.Equal(t, uint64(2), binary.LittleEndian.Uint64(header[0x10:]))
}

// The output file may be on the disk being carved, so chunks we
// wrote can be found again by the scan.
func TestEvtxWriterSkipsOwnOutput(t *testing.T) {
	others := otherLogChunks(t)

	out := &memWriter{}
	writer, err := newEvtxWriter(out)
	assert.NoError(t, err)

	// Written out of order so the second chunk is renumbered.
	for _, c := range []*carvedChunk{others[1], others[0]} {
		ok, err := writer.WriteChunk(c)
		assert.NoError(t, err)
		assert.True(t, ok)
	}

	// The second chunk was renumbered when written, so it differs
	// from the original.
	off := fileHeaderSize + evtx.EVTX_CHUNK_SIZE
	found_again := mustCheckChunk(t, append([]byte{}, out.buf[off:off+evtx.EVTX_CHUNK_SIZE]...))
	assert.NotEqual(t, others[0].Data, found_again.Data)

	ok, err := writer.WriteChunk(found_again)
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, uint64(2), writer.Chunks())
}

func TestEvtxWriterEmpty(t *testing.T) {
	out := &memWriter{}
	writer, err := newEvtxWriter(out)
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())
	assert.Equal(t, fileHeaderSize, len(out.buf))
	assert.Equal(t, evtx.EVTX_HEADER_MAGIC, string(out.buf[:8]))
}

func newRng() *rand.Rand {
	return rand.New(rand.NewSource(11))
}
