package utils

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"time"

	"www.velocidex.com/golang/velociraptor/constants"
)

var (
	generator FlowIdGenerator = RandomFlowIdGenerator{}
)

type FlowIdGenerator interface {
	Next(client_id string) string
}

type RandomFlowIdGenerator struct{}

func (self RandomFlowIdGenerator) Next(client_id string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.FLOW_PREFIX + result

}

type ConstantFlowIdGenerator string

func (self ConstantFlowIdGenerator) Next(client_id string) string {
	return string(self)
}

type IncrementalFlowIdGenerator int

func (self *IncrementalFlowIdGenerator) Next(client_id string) string {
	*self = *self + 1
	return fmt.Sprintf("F.%d", *self)
}

func NewFlowId(client_id string) string {
	return generator.Next(client_id)
}

func SetFlowIdGenerator(gen FlowIdGenerator) {
	generator = gen
}
