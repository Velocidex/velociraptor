//go:generate bash -c "binparsegen conversion.spec.yaml > profile_gen.go"

package ese

import (
	"fmt"
	"math/bits"
)

// https://devblogs.microsoft.com/oldnewthing/20040315-00/?p=40253

func (self *SID) String() string {
	result := fmt.Sprintf("S-%d-%d", self.Revision(),
		uint64(bits.ReverseBytes16(self.Authority()))<<32+
			uint64(bits.ReverseBytes32(self.Authority2())))

	sub_authorities := self.Subauthority()
	for i := 0; i < int(self.SubAuthCount()); i++ {
		if i > len(sub_authorities) {
			break
		}

		sub := sub_authorities[i]
		result += fmt.Sprintf("-%d", sub)
	}
	return result
}
