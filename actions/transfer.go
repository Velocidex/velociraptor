package actions

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/responder"
)

type TransferBuffer struct{}

func (self *TransferBuffer) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)
	arg, pres := responder.GetArgs().(*actions_proto.BufferReference)
	if !pres {
		responder.RaiseError("Request should be of type BufferReference")
		return
	}
	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	accessor := glob.OSFileSystemAccessor{}
	file, err := accessor.Open(*path)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}
	defer file.Close()

	_, err = file.Seek(int64(arg.Offset), 0)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	if arg.Length > 1000000 {
		responder.RaiseError("Unable to read such a large buffer.")
		return
	}
	buffer := make([]byte, arg.Length)
	bytes_read, err := file.Read(buffer)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(buffer)
	w.Close()

	responder.SendResponseToWellKnownFlow(
		"aff4:/flows/F:TransferStore",
		&actions_proto.DataBlob{
			Data:        b.Bytes(),
			Compression: actions_proto.DataBlob_ZCOMPRESSION,
		},
	)

	hash := sha256.Sum256(buffer)
	responder.AddResponse(&actions_proto.BufferReference{
		Offset:   arg.Offset,
		Length:   uint64(bytes_read),
		Data:     hash[:],
		Pathspec: arg.Pathspec,
	})

	responder.Return()
}
