package responder

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

// Keeps track of flow statistics to update the server on flow
// progress.

type Stats struct {
	*crypto_proto.FlowStats

	mu            sync.Mutex
	frequency_sec uint64
}

// Inspect the messages and update the flow stats based on them.
func (self *Stats) UpdateStats(message *crypto_proto.VeloMessage) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if message.LogMessage != nil {
		self.TotalLogs += message.LogMessage.NumberOfRows
		return
	}

	if message.FileBuffer != nil {
		if message.FileBuffer.Offset == 0 {
			message.FileBuffer.UploadNumber = int64(self.TotalUploadedFiles)

			self.TotalUploadedFiles++
			self.TotalExpectedUploadedBytes += message.FileBuffer.StoredSize
			return
		}
		self.TotalUploadedBytes += uint64(len(message.FileBuffer.Data))
		return
	}

	if message.VQLResponse != nil {
		self.TotalCollectedRows += message.VQLResponse.TotalRows
		return
	}

	if message.Status != nil {
		self.QueryStatus = append(self.QueryStatus,
			proto.Clone(message.Status).(*crypto_proto.VeloStatus))
		return
	}
}

func (self *Stats) GetStats() *crypto_proto.FlowStats {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(self.FlowStats).(*crypto_proto.FlowStats)
}

// Returns some stats to send to the server. The stats are sent in a
// rate limited way - not too frequently.
func (self *Stats) MaybeSendStats() *crypto_proto.FlowStats {
	self.mu.Lock()
	defer self.mu.Unlock()

	now := uint64(time.Now().Unix())
	if now-self.Timestamp > self.frequency_sec {
		self.Timestamp = now
		return proto.Clone(self.FlowStats).(*crypto_proto.FlowStats)
	}
	return nil
}
