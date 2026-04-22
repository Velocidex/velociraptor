package allocs

import (
	"unsafe"

	"www.velocidex.com/golang/velociraptor/utils"
)

// Allocates memory with 8 byte alignment
func AllocateAlignedBuff(length int) []byte {
	buffer := make([]byte, length+8)
	offset := int(uintptr(unsafe.Pointer(&buffer[0])) & uintptr(0xF))

	return buffer[offset:]
}

func AllocUint64(length, max_length int) ([]uint64, error) {
	if length*8 > max_length {
		return nil, utils.MemoryError
	}

	return make([]uint64, length), nil
}
