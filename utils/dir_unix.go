// +build linux darwin freebsd

package utils

import (
	"io"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

// Auxiliary information if the File describes a directory
type dirInfo struct {
	buf  []byte // buffer for directory I/O
	nbuf int    // length of buf; return value from Getdirentries
	bufp int    // location of next record in buf.
}

const (
	// More than 5760 to work around https://golang.org/issue/24015.
	blockSize = 8192
)

// copied and stripped-down variant of os.File.readdir which only returns
// filenames and also includes entries whose inode number is 0
func readdirnames(f *os.File, n int) (names []string, err error) {
	// If this file has no dirinfo, create one.
	dirinfo := new(dirInfo)
	dirinfo.buf = make([]byte, blockSize)

	// Change the meaning of n for the implementation below.
	//
	// The n above was for the public interface of "if n <= 0,
	// Readdir returns all the FileInfo from the directory in a
	// single slice".
	//
	// But below, we use only negative to mean looping until the
	// end and positive to mean bounded, with positive
	// terminating at 0.
	if n == 0 {
		n = -1
	}

	for n != 0 {
		// Refill the buffer if necessary
		if dirinfo.bufp >= dirinfo.nbuf {
			dirinfo.bufp = 0
			var errno error
			dirinfo.nbuf, errno = readDirent(f, dirinfo.buf)
			runtime.KeepAlive(f)
			if errno != nil {
				return names, &os.PathError{Op: "readdirent", Path: f.Name(), Err: errno}
			}
			if dirinfo.nbuf <= 0 {
				break // EOF
			}
		}

		// Drain the buffer
		buf := dirinfo.buf[dirinfo.bufp:dirinfo.nbuf]
		reclen, ok := direntReclen(buf)
		if !ok || reclen > uint64(len(buf)) {
			break
		}
		rec := buf[:reclen]
		dirinfo.bufp += int(reclen)

		const namoff = uint64(unsafe.Offsetof(syscall.Dirent{}.Name))
		namlen, ok := direntNamlen(rec)
		if !ok || namoff+namlen > uint64(len(rec)) {
			break
		}
		name := rec[namoff : namoff+namlen]
		for i, c := range name {
			if c == 0 {
				name = name[:i]
				break
			}
		}
		// Check for useless names before allocating a string.
		if string(name) == "." || string(name) == ".." {
			continue
		}
		if n > 0 { // see 'n == 0' comment above
			n--
		}
		names = append(names, string(name))
	}

	if n > 0 && len(names) == 0 {
		return nil, io.EOF
	}
	return names, nil
}

// readDirent wraps syscall.ReadDirent.
// We treat this like an ordinary system call rather than a call
// that tries to fill the buffer.
func readDirent(fd *os.File, buf []byte) (int, error) {
	for {
		n, err := syscall.ReadDirent(int(fd.Fd()), buf)

		// ignore EAGAIN and EINTR -- this might cause issues
		// when the syscall is interrupted repeatedly
		if err == syscall.EAGAIN || err == syscall.EINTR {
			continue
		}
		// Do not call eofError; caller does not expect to see io.EOF.
		return n, err
	}
}

func direntReclen(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Reclen), unsafe.Sizeof(syscall.Dirent{}.Reclen))
}

func direntNamlen(buf []byte) (uint64, bool) {
	reclen, ok := direntReclen(buf)
	if !ok {
		return 0, false
	}
	return reclen - uint64(unsafe.Offsetof(syscall.Dirent{}.Name)), true
}

// readInt returns the size-bytes unsigned integer in native byte order at offset off.
func readInt(b []byte, off, size uintptr) (u uint64, ok bool) {
	if len(b) < int(off+size) {
		return 0, false
	}
	if isBigEndian {
		return readIntBE(b[off:], size), true
	}
	return readIntLE(b[off:], size), true
}

func readIntBE(b []byte, size uintptr) uint64 {
	switch size {
	case 1:
		return uint64(b[0])
	case 2:
		_ = b[1] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[1]) | uint64(b[0])<<8
	case 4:
		_ = b[3] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[3]) | uint64(b[2])<<8 | uint64(b[1])<<16 | uint64(b[0])<<24
	case 8:
		_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 | uint64(b[4])<<24 |
			uint64(b[3])<<32 | uint64(b[2])<<40 | uint64(b[1])<<48 | uint64(b[0])<<56
	default:
		panic("syscall: readInt with unsupported size")
	}
}

func readIntLE(b []byte, size uintptr) uint64 {
	switch size {
	case 1:
		return uint64(b[0])
	case 2:
		_ = b[1] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[0]) | uint64(b[1])<<8
	case 4:
		_ = b[3] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
	case 8:
		_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
			uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
	default:
		panic("syscall: readInt with unsupported size")
	}
}
