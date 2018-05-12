package actions

import (
	"crypto/sha1"
	"crypto/sha256"
	"github.com/golang/protobuf/proto"
	"os"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type HashBuffer struct{}

func (self *HashBuffer) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := NewResponder(msg, output)
	arg, pres := responder.GetArgs().(*actions_proto.BufferReference)
	if !pres {
		responder.RaiseError("Request should be of type FingerprintRequest")
		return
	}
	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	file, err := os.Open(*path)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	_, err = file.Seek(int64(*arg.Offset), 0)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	if *arg.Length > 1000000 {
		responder.RaiseError("Unable to hash such a large buffer.")
		return
	}
	buffer := make([]byte, *arg.Length)
	bytes_read, err := file.Read(buffer)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	hash := sha256.Sum256(buffer)

	responder.AddResponse(&actions_proto.BufferReference{
		Offset:   arg.Offset,
		Length:   proto.Uint64(uint64(bytes_read)),
		Data:     hash[:],
		Pathspec: arg.Pathspec,
	})

	responder.Return()
}

type HashFile struct{}

func (self *HashFile) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := NewResponder(msg, output)

	arg, pres := responder.GetArgs().(*actions_proto.FingerprintRequest)
	if !pres {
		responder.RaiseError("Request should be of type FingerprintRequest")
		return
	}
	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	file, err := os.Open(*path)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	buffer := make([]byte, 1000000)
	response := actions_proto.FingerprintResponse{
		Pathspec: arg.Pathspec,
	}
	for _, tuple := range arg.Tuples {
		if *tuple.FpType == *actions_proto.FingerprintTuple_FPT_GENERIC.Enum() {
			sha1_hash := sha1.New()
			sha256_hash := sha256.New()
			hash := &actions_proto.Hash{}
			var total uint64
			for {
				bytes_read, err := file.Read(buffer)
				if err != nil {
					responder.RaiseError(err.Error())
					return
				}

				total += uint64(bytes_read)
				sha1_hash.Write(buffer)
				sha256_hash.Write(buffer)
				if bytes_read < len(buffer) || total > *arg.MaxFilesize {
					hash.Sha256 = sha256_hash.Sum(nil)
					hash.Sha1 = sha1_hash.Sum(nil)
					hash.NumBytes = &total
					response.Hash = hash
					break
				}
			}
		}
	}
	if response.Hash != nil {
		responder.AddResponse(&response)
	}

	responder.Return()
}
