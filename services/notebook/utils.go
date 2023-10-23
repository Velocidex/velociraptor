package notebook

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"strings"

	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	testMode bool
)

func SetTestMode() func() {
	testMode = true
	return func() {
		testMode = false
	}
}

func getRand(buf []byte) {
	if testMode {
		b := strings.NewReader("XXXX112342")
		b.Read(buf)
		return
	}
	rand.Read(buf)
}

func NewNotebookId() string {
	buf := make([]byte, 8)
	getRand(buf)

	binary.BigEndian.PutUint32(buf, uint32(utils.GetTime().Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "N." + result
}

func NewNotebookCellId() string {
	buf := make([]byte, 8)
	getRand(buf)

	binary.BigEndian.PutUint32(buf, uint32(utils.GetTime().Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "NC." + result
}

func NewNotebookAttachmentId() string {
	buf := make([]byte, 8)
	getRand(buf)

	binary.BigEndian.PutUint32(buf, uint32(utils.GetTime().Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "NA." + result
}
