package utils

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var (
	generator     IdGenerator = RandomIdGenerator{}
	generator_mu  sync.Mutex
	generator_set string
)

type IdGenerator interface {
	Next(client_id string) string
}

type RandomIdGenerator struct{}

func (self RandomIdGenerator) Next(client_id string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return result
}

type ConstantIdGenerator string

func (self ConstantIdGenerator) Next(client_id string) string {
	return string(self)
}

type IncrementalIdGenerator int

func (self *IncrementalIdGenerator) Next(client_id string) string {
	*self = *self + 1
	return fmt.Sprintf("%02d", *self)
}

func NextId() string {
	return generator.Next("")
}

func NewFlowId(client_id string) string {
	next := generator.Next(client_id)
	if !strings.HasPrefix(next, "F.") {
		return "F." + next
	}
	return next
}

func SetFlowIdForTests(id string) func() {
	return SetIdGenerator(ConstantIdGenerator(id))
}

// This is only called from tests! It resets the global id generator
// in order to get reproducible IDs for various things. It is
// important to ensure the ID generator is only reset once by a single
// calling thread. Otherwise this introduces test flakeyness. We
// ensure this happens by hard panicing if it is called multiple
// times. Tests should be refactored to only call this once from the
// main thread, and call the closer function when done.
func SetIdGenerator(gen IdGenerator) func() {
	if generator_set != "" {
		panic("ID Generator already set: Previous call \n" + generator_set)
	}

	generator_mu.Lock()

	old_gen := generator
	generator = gen
	generator_set = string(debug.Stack())

	return func() {
		generator = old_gen
		generator_set = ""
		generator_mu.Unlock()
	}
}
