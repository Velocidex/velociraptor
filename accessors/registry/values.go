//go:build windows
// +build windows

package registry

import (
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows/registry"
	"www.velocidex.com/golang/velociraptor/vql/windows"
)

const (
	_ERROR_NO_MORE_ITEMS syscall.Errno = 259
)

var pool = sync.Pool{
	New: func() interface{} {
		buffer := make([]byte, 1024)
		return &buffer
	},
}

// A more optimized key.GetValue() - avoid unnecessary syscalls and allocations
func getValue(key registry.Key, value_name string) (
	buf_size int, value_type uint32, value interface{}, err error) {

	metricsReadValue.Inc()

	// Use the pool to avoid allocations.
	cached_buffer := pool.Get().(*[]byte)
	defer pool.Put(cached_buffer)

	data := *cached_buffer

	buf_size, value_type, err = key.GetValue(value_name, data)
	if err == syscall.ERROR_MORE_DATA {

		// Try again with larger buffer.
		buf := make([]byte, buf_size)
		buf_size, value_type, err = key.GetValue(value_name, buf)
		if err != nil {
			return buf_size, value_type, "", err
		}
		data = buf
	}
	if err != nil {
		return buf_size, value_type, "", err
	}

	// Now parse the value based on the type.
	// Following code is based on https://cs.opensource.google/go/x/sys/+/refs/tags/v0.18.0:windows/registry/value.go
	switch value_type {
	case registry.DWORD:
		if buf_size == 4 {
			var val32 uint32
			copy((*[4]byte)(unsafe.Pointer(&val32))[:], data)
			return buf_size, value_type, uint64(val32), nil
		}

	case registry.QWORD:
		if buf_size == 8 {
			var val64 uint64
			copy((*[8]byte)(unsafe.Pointer(&val64))[:], data)
			return buf_size, value_type, uint64(val64), nil
		}

	case registry.BINARY:
		// Need to make a copy of the data so the buffer may be
		// returned to the pool.
		new_buff := append([]byte{}, data[:buf_size]...)
		return buf_size, value_type, new_buff, nil

		// We deliberately do not expand this because it depends on
		// the process env.
	case registry.SZ, registry.EXPAND_SZ:
		u := (*[1 << 29]uint16)(unsafe.Pointer(&data[0]))[: len(data)/2 : len(data)/2]
		return buf_size, value_type, syscall.UTF16ToString(u), nil

	case registry.MULTI_SZ:
		u := (*[1 << 29]uint16)(unsafe.Pointer(&data[0]))[: len(data)/2 : len(data)/2]
		parts := strings.Split(string(utf16.Decode(u)), "\x00")
		res := []string{}
		for _, p := range parts {
			if p != "" {
				res = append(res, p)
			}
		}
		return buf_size, value_type, res, nil

	default:
	}

	// Otherwise just return the binary buffer.
	new_buff := append([]byte{}, data[:buf_size]...)
	return buf_size, value_type, new_buff, nil
}

func ReadValueNames(k registry.Key) ([]string, error) {
	ki, err := k.Stat()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, ki.ValueCount)
	// Use the pool to avoid allocations.
	cached_buffer := pool.Get().(*[]byte)
	defer pool.Put(cached_buffer)

	buf := *cached_buffer

loopItems:
	for i := uint32(0); ; i++ {
		l := uint32(len(buf)) / 2
		for {
			err := windows.RegEnumValue(syscall.Handle(k), i, &buf[0], &l, nil, nil, nil, nil)
			if err == nil {
				break
			}
			if err == syscall.ERROR_MORE_DATA {
				// Double buffer size and try again.
				buf = make([]byte, 2*len(buf))
				l = uint32(len(buf)) / 2
				continue
			}

			if err == _ERROR_NO_MORE_ITEMS {
				break loopItems
			}

			return names, err
		}

		u := (*[1 << 29]uint16)(unsafe.Pointer(&buf[0]))[: len(buf)/2 : len(buf)/2]
		names = append(names, syscall.UTF16ToString(u))
	}
	return names, nil
}
