package responder

import (
	"sync"

	"google.golang.org/protobuf/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
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
		self.TotalUploadedBytes += uint64(len(message.FileBuffer.Data))

		if message.FileBuffer.Offset == 0 {
			message.FileBuffer.UploadNumber = int64(self.TotalUploadedFiles)

			self.TotalUploadedFiles++
			self.TotalExpectedUploadedBytes += message.FileBuffer.StoredSize
			return
		}
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

func (self *Stats) SendFinalFlowStats(responder *Responder) {
	self.mu.Lock()
	flow_complete := self.FlowComplete
	self.FlowComplete = true
	flow_stats := proto.Clone(self.FlowStats).(*crypto_proto.FlowStats)
	self.mu.Unlock()

	if !flow_complete {
		responder.AddResponse(&crypto_proto.VeloMessage{
			RequestId: constants.STATS_SINK,
			FlowStats: flow_stats,
		})
	}
}

func (self *Stats) Get() *crypto_proto.FlowStats {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(self.FlowStats).(*crypto_proto.FlowStats)
}

// Returns some stats to send to the server. The stats are sent in a
// rate limited way - not too frequently.
func (self *Stats) MaybeSendStats() *crypto_proto.FlowStats {
	self.mu.Lock()
	defer self.mu.Unlock()

	now := uint64(utils.GetTime().Now().UnixNano() / 1000)
	last_timestamp := self.Timestamp
	self.Timestamp = now

	if now-last_timestamp > self.frequency_sec && !self.FlowComplete {
		return proto.Clone(self.FlowStats).(*crypto_proto.FlowStats)
	}
	return nil
}
