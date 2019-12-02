//go:generate bash -c "binparsegen conversion.spec.yaml > profile_gen.go"

package ese

import (
	"fmt"
	"math/bits"
)

func (self *SID) String() string {
	result := fmt.Sprintf("S-%d", uint64(bits.ReverseBytes16(self.Authority()))<<32+
		uint64(bits.ReverseBytes32(self.Authority2())))

	for _, sub := range self.Subauthority() {
		if sub != 0 {
			result += fmt.Sprintf("-%d", sub)
		}
	}

	return result
}
