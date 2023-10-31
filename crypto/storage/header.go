package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"www.velocidex.com/golang/velociraptor/utils"
)

// This is an at-rest crypto storage file.
type Header struct {
	Magic               [8]byte
	LastUpdateTimestamp uint64

	// Offset to the first message
	FirstMessage uint64

	Next uint64
}

func (self *Header) Read(fd io.ReaderAt) error {
	buf := make([]byte, binary.Size(*self))
	n, err := fd.ReadAt(buf, 0)
	if err != nil {
		return err
	}
	return binary.Read(bytes.NewReader(buf[:n]),
		binary.LittleEndian, self)
}

func (self *Header) Write(fd io.WriterAt) error {
	self.Magic = MAGIC
	self.LastUpdateTimestamp = uint64(utils.GetTime().Now().UnixNano())

	b := &bytes.Buffer{}
	err := binary.Write(b, binary.LittleEndian, self)
	if err != nil {
		return err
	}

	_, err = fd.WriteAt(b.Bytes(), 0)
	return err
}

type Message struct {
	// The type of the message
	Type [8]byte

	// This is the data within this message packet
	Start  uint64
	Length uint32

	// Offset to the next message or 0 for no more messages.
	Next uint64
}

func (self *Message) ReadAt(fd io.ReaderAt, offset uint64) error {
	buf := make([]byte, binary.Size(*self))
	n, err := fd.ReadAt(buf, int64(offset))
	if err != nil {
		return err
	}
	return binary.Read(bytes.NewReader(buf[:n]),
		binary.LittleEndian, self)
}

// Represents the public key in PEM format of the server required to
// decrypt this file.
type PublicKey struct {
	Message

	Pem []byte
}

func (self *PublicKey) WriteAt(fd io.WriterAt, offset int64) error {
	if len(self.Pem) == 0 {
		return errors.New("Server PEM not initialized")
	}

	self.Message.Type = PUB_KEY_TYPE
	self.Message.Start = uint64(offset) + uint64(binary.Size(self.Message))
	self.Message.Length = uint32(len(self.Pem))
	self.Message.Next = self.Message.Start + uint64(self.Message.Length)

	b := &bytes.Buffer{}
	err := binary.Write(b, binary.LittleEndian, self.Message)
	if err != nil {
		return err
	}

	_, err = b.Write(self.Pem)
	if err != nil {
		return err
	}

	_, err = fd.WriteAt(b.Bytes(), offset)
	return err
}

type Packet struct {
	Message

	Data []byte
}

func (self *Packet) WriteAt(fd io.WriterAt, offset int64) error {
	self.Message.Type = PACKET_TYPE
	self.Message.Start = uint64(offset) + uint64(binary.Size(self.Message))
	self.Message.Length = uint32(len(self.Data))
	self.Message.Next = self.Message.Start + uint64(self.Message.Length)

	b := &bytes.Buffer{}
	err := binary.Write(b, binary.LittleEndian, self.Message)
	if err != nil {
		return err
	}

	_, err = b.Write(self.Data)
	if err != nil {
		return err
	}

	_, err = fd.WriteAt(b.Bytes(), offset)
	return err
}

// The Client's certificate is used to verify signed messages.
type ClientCSR struct {
	Message

	Pem []byte
}

func (self *ClientCSR) WriteAt(fd io.WriterAt, offset int64) error {
	self.Message.Type = CLIENT_CSR
	self.Message.Start = uint64(offset) + uint64(binary.Size(self.Message))
	self.Message.Length = uint32(len(self.Pem))
	self.Message.Next = self.Message.Start + uint64(self.Message.Length)

	b := &bytes.Buffer{}
	err := binary.Write(b, binary.LittleEndian, self.Message)
	if err != nil {
		return err
	}

	_, err = b.Write(self.Pem)
	if err != nil {
		return err
	}

	_, err = fd.WriteAt(b.Bytes(), offset)
	return err
}
