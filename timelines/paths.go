package timelines

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"time"

	"www.velocidex.com/golang/velociraptor/constants"
)

type TimelinePathManager struct {
	Name  string
	Super string
}

func (self TimelinePathManager) Path() string {
	return constants.TIMELINE_URN + self.Super + "/" + self.Name + ".json"
}
func (self TimelinePathManager) Index() string {
	return constants.TIMELINE_URN + self.Super + "/" + self.Name + ".idx"
}

func NewTimelineId() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "T." + result
}
