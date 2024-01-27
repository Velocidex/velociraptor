package utils

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

var (
	generator IdGenerator = RandomIdGenerator{}
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

func SetIdGenerator(gen IdGenerator) {
	generator = gen
}
