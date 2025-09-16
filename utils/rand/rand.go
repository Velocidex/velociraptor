// As of Go 1.24 rand has been changed in a backwards incompatible
// way. This module is a compatibility module.
package rand

import "math/rand"

var (
	Disabled bool
)

func DisableRand() (closer func()) {
	Disabled = true
	return func() {
		Disabled = false
	}
}

func Read(b []byte) (n int, err error) {
	return rand.Read(b)
}

// As of Go 1.24 [Seed] is a no-op. To restore the previous behavior
// set GODEBUG=randseednop=0.
func Seed(in int64) {}

func Intn(in int) int {
	if Disabled {
		return 0
	}
	return rand.Intn(in)
}

func Shuffle(len int, cb func(i, j int)) {
	if !Disabled {
		rand.Shuffle(len, cb)
	}
}
