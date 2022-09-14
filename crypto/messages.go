package crypto

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Once a message is decoded the MessageInfo contains metadata about it.
type MessageInfo struct {
	// The compressed MessageList protobufs sent in each POST.
	RawCompressed [][]byte
	Authenticated bool
	Source        string
	RemoteAddr    string
	Compression   crypto_proto.PackedMessageList_CompressionType
	OrgId         string
}

// Apply the callback on each job message. This saves memory since we
// immediately use the decompressed buffer and not hold it around.
func (self *MessageInfo) IterateJobs(
	ctx context.Context,
	processor func(ctx context.Context, msg *crypto_proto.VeloMessage)) error {
	for _, raw := range self.RawCompressed {
		if self.Compression == crypto_proto.PackedMessageList_ZCOMPRESSION {
			decompressed, err := utils.Uncompress(ctx, raw)
			if err != nil {
				return errors.New("Unable to decompress MessageList")
			}
			raw = decompressed
		}

		message_list := &crypto_proto.MessageList{}
		err := proto.Unmarshal(raw, message_list)
		if err != nil {
			// TODO: Issue #2069 suggests that maybe the message is
			// compressed but it is not indicated as such under as yet
			// unknown conditions. We retry to decompress the data
			// anyway just in case to see if the data is actually
			// maybe compressed twice or just marked as uncompressed
			// incorrectly. The proto unmarshal error should never
			// really occur so the performance hit on this is not
			// expected to be large.
			decompressed, err2 := utils.Uncompress(ctx, raw)
			if err2 != nil {
				// Zlib decompression does have a checksum so if we
				// hit an error here just pass the original protobuf
				// errr on.
				return err
			}

			// Try to parse it with the decompressed data just in
			// case.
			err := proto.Unmarshal(decompressed, message_list)
			if err != nil {
				// No it really is corrupted :-( no idea how this
				// happens...
				return errors.WithStack(err)
			}
		}

		for _, job := range message_list.Job {
			if self.Authenticated {
				job.AuthState = crypto_proto.VeloMessage_AUTHENTICATED
			}
			job.Source = self.Source
			job.OrgId = self.OrgId

			// For backwards compatibility normalize old
			// client messages to new format.
			err = responder.NormalizeVeloMessageForBackwardCompatibility(job)
			if err != nil {
				return err
			}

			processor(ctx, job)
		}
	}

	return nil
}
