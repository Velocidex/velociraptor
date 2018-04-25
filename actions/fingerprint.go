package actions

import (
	"os"
	"crypto/sha1"
	"crypto/sha256"
	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type HashBuffer struct{}
func (self *HashBuffer) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(msg)
	arg, pres := responder.GetArgs().(*actions_proto.BufferReference)
	if !pres {
		return responder.RaiseError("Request should be of type FingerprintRequest")
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
		return responder.RaiseError("Unable to hash such a large buffer.")
	}
	buffer := make([]byte, *arg.Length)
	bytes_read, err := file.Read(buffer)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	hash := sha256.Sum256(buffer)

	responder.AddResponse(&actions_proto.BufferReference{
		Offset: arg.Offset,
		Length: proto.Uint64(uint64(bytes_read)),
		Data: hash[:],
		Pathspec: arg.Pathspec,
	})

	return responder.Return()
}



type HashFile struct{}

func (self *HashFile) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(msg)

	arg, pres := responder.GetArgs().(*actions_proto.FingerprintRequest)
	if !pres {
		return responder.RaiseError("Request should be of type FingerprintRequest")
	}
	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	file, err := os.Open(*path)
	if err != nil {
		return responder.RaiseError(err.Error())
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
					return responder.RaiseError(err.Error())
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

	return responder.Return()
}
