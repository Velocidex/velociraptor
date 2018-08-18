//
package binary

import (
	"encoding/binary"
)

func AddModel(profile *Profile) {

	profile.types["unsigned long long"] = NewIntParser(
		"unsigned long long",
		func(buf []byte) uint64 {
			return uint64(binary.LittleEndian.Uint64(buf))
		})
	profile.types["unsigned short"] = NewIntParser(
		"unsigned short", func(buf []byte) uint64 {
			return uint64(binary.LittleEndian.Uint16(buf))
		})
	profile.types["int8"] = NewIntParser(
		"int8", func(buf []byte) uint64 {
			return uint64(buf[0])
		})
	profile.types["int16"] = NewIntParser(
		"int16", func(buf []byte) uint64 {
			return uint64(binary.LittleEndian.Uint16(buf))
		})
	profile.types["int32"] = NewIntParser(
		"int32", func(buf []byte) uint64 {
			return uint64(binary.LittleEndian.Uint32(buf))
		})
	profile.types["String"] = NewStringParser("string")
	profile.types["Enumeration"] = NewEnumeration("Enumeration", profile)

	profile.types["Array"] = NewArrayParser("Array", "", profile, nil)

	// Aliases
	profile.types["int"] = profile.types["int32"]
	profile.types["char"] = profile.types["int8"]
	profile.types["short int"] = profile.types["int16"]
}
