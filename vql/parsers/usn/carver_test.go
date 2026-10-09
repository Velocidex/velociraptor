package usn

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

// putUSNRecordV2 writes a USN_RECORD_V2 at offset and returns its length.
func putUSNRecordV2(buf []byte, offset int64, mft_id uint64, name string) int64 {
	utf16 := make([]byte, 0, 2*len(name))
	for _, c := range name {
		utf16 = append(utf16, byte(c), 0)
	}
	length := (60 + int64(len(utf16)) + 7) &^ 7

	rec := buf[offset : offset+length]
	binary.LittleEndian.PutUint32(rec[0:], uint32(length))
	binary.LittleEndian.PutUint16(rec[4:], 2) // MajorVersion
	binary.LittleEndian.PutUint64(rec[8:], mft_id|1<<48)
	binary.LittleEndian.PutUint64(rec[16:], 5|1<<48)
	binary.LittleEndian.PutUint64(rec[24:], uint64(offset))

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	binary.LittleEndian.PutUint64(rec[32:], uint64(ts.Unix()+11644473600)*10000000)
	binary.LittleEndian.PutUint32(rec[40:], 0x100) // USN_REASON_FILE_CREATE
	binary.LittleEndian.PutUint16(rec[56:], uint16(len(utf16)))
	binary.LittleEndian.PutUint16(rec[58:], 60)
	copy(rec[60:], utf16)

	return length
}

// carve runs carve_usn with args and returns the rows and the log.
func carve(t *testing.T, args *ordereddict.Dict) ([]vfilter.Row, string) {
	log_buffer := &bytes.Buffer{}
	scope := vql_subsystem.MakeScope()
	scope.AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(log_buffer, "", 0))
	defer scope.Close()

	rows := []vfilter.Row{}
	for row := range (CarveUSNPlugin{}).Call(context.Background(), scope, args) {
		rows = append(rows, row)
	}
	return rows, log_buffer.String()
}

// writeRange copies a stream from the NTFS image to a file.
func writeRange(t *testing.T, path string, rng ntfs.RangeReaderAt) {
	data := make([]byte, ntfs.RangeSize(rng))
	_, err := rng.ReadAt(data, 0)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(path, data, 0644))
}

func TestCarveUSNFromFiles(t *testing.T) {
	dir := t.TempDir()

	// A $J dump with three records at 16 byte aligned offsets.
	journal := make([]byte, 0x4000)
	offsets := []int64{0x100, 0x1000, 0x2010}
	for i, off := range offsets {
		putUSNRecordV2(journal, off, uint64(100+i), "carved.txt")
	}
	usn_path := filepath.Join(dir, "J")
	assert.NoError(t, os.WriteFile(usn_path, journal, 0644))

	// A raw $MFT dump from the test image (it has no USN journal).
	fd, err := os.Open("../../../artifacts/testdata/files/test.ntfs.dd")
	assert.NoError(t, err)
	defer fd.Close()

	ntfs_ctx, err := ntfs.GetNTFSContext(fd, 0)
	assert.NoError(t, err)

	mft_entry, err := ntfs_ctx.GetMFT(0)
	assert.NoError(t, err)
	mft_stream, err := ntfs.OpenStream(ntfs_ctx, mft_entry,
		ntfs.ATTR_TYPE_DATA, ntfs.WILDCARD_STREAM_ID, ntfs.WILDCARD_STREAM_NAME)
	assert.NoError(t, err)
	mft_path := filepath.Join(dir, "MFT")
	writeRange(t, mft_path, mft_stream)

	disk_offsets := func(rows []vfilter.Row) []int64 {
		result := []int64{}
		for _, row := range rows {
			off, _ := row.(*ordereddict.Dict).Get("DiskOffset")
			result = append(result, off.(int64))
		}
		return result
	}

	// usn_filename alone used to dereference a nil NTFS context.
	rows, logs := carve(t, ordereddict.NewDict().
		Set("accessor", "file").
		Set("usn_filename", usn_path))
	assert.NotContains(t, logs, "PANIC")
	assert.Equal(t, offsets, disk_offsets(rows), logs)

	// mft_filename + usn_filename used to carve nothing: the USN file
	// was never opened and the size was 0.
	rows, logs = carve(t, ordereddict.NewDict().
		Set("accessor", "file").
		Set("mft_filename", mft_path).
		Set("usn_filename", usn_path))
	assert.NotContains(t, logs, "PANIC")
	assert.NotContains(t, logs, "with size 0")
	assert.Equal(t, offsets, disk_offsets(rows), logs)
}
