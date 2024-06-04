package utils

import "unsafe"

// Allocates memory with 8 byte alignment
func AllocateBuff(length int) []byte {
	buffer := make([]byte, length+8)
	offset := int(uintptr(unsafe.Pointer(&buffer[0])) & uintptr(0xF))

	return buffer[offset:]
}
