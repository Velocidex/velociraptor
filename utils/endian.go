package utils

import (
	"runtime"
)

var isBigEndian bool

func init() {
	switch runtime.GOARCH {
	case "386", "amd64", "arm", "arm64", "ppc64le", "mips64le", "mipsle", "riscv64", "wasm":
		isBigEndian = false
	case "ppc64", "s390x", "mips", "mips64":
		isBigEndian = true
	default:
		// nop
	}
}