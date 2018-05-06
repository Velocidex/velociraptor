package actions

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"github.com/golang/protobuf/proto"
	"os"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type TransferBuffer struct{}

func (self *TransferBuffer) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(msg)
	arg, pres := responder.GetArgs().(*actions_proto.BufferReference)
	if !pres {
		return responder.RaiseError("Request should be of type BufferReference")
	}
	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	file, err := os.Open(*path)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	_, err = file.Seek(int64(*arg.Offset), 0)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	if *arg.Length > 1000000 {
		return responder.RaiseError("Unable to read such a large buffer.")
	}
	buffer := make([]byte, *arg.Length)
	bytes_read, err := file.Read(buffer)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(buffer)
	w.Close()

	responder.SendResponseToWellKnownFlow(
		"aff4:/flows/F:TransferStore",
		&actions_proto.DataBlob{
			Data:        b.Bytes(),
			Compression: actions_proto.DataBlob_ZCOMPRESSION.Enum(),
		},
	)

	hash := sha256.Sum256(buffer)
	responder.AddResponse(&actions_proto.BufferReference{
		Offset:   arg.Offset,
		Length:   proto.Uint64(uint64(bytes_read)),
		Data:     hash[:],
		Pathspec: arg.Pathspec,
	})

	return responder.Return()
}
