//
package binary

import (
	"encoding/binary"
)

func AddModel(profile *Profile) {
	profile.types["unsigned long long"] = &IntParser{
		name: "unsigned long long",
		converter: func(buf []byte) uint64 {
			return uint64(binary.LittleEndian.Uint64(buf))
		},
	}
	profile.types["unsigned short"] = &IntParser{
		name: "unsigned short",
		converter: func(buf []byte) uint64 {
			return uint64(binary.LittleEndian.Uint16(buf))
		},
	}
	profile.types["int8"] = &IntParser{
		name: "int8",
		converter: func(buf []byte) uint64 {
			return uint64(buf[0])
		},
	}
	profile.types["int16"] = &IntParser{
		name: "int16",
		converter: func(buf []byte) uint64 {
			return uint64(binary.LittleEndian.Uint16(buf))
		},
	}
	profile.types["int32"] = &IntParser{
		name: "int32",
		converter: func(buf []byte) uint64 {
			return uint64(binary.LittleEndian.Uint32(buf))
		},
	}
	profile.types["String"] = &StringParser{
		type_name: "string",
	}

	profile.types["Enumeration"] = &Enumeration{
		profile:   profile,
		type_name: "Enumeration",
	}

	// Aliases
	profile.types["int"] = profile.types["int32"]
	profile.types["char"] = profile.types["int8"]
	profile.types["short int"] = profile.types["int16"]
}
