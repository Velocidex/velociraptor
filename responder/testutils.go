package responder

import (
	"context"
	"sync"
	"testing"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type TestResponderType struct {
	*FlowResponder
	Drain *messageDrain
}

func (self *TestResponderType) Close() {
	self.FlowResponder.Close()
	self.FlowResponder.flow_context.Close()
}

func (self *TestResponderType) Output() chan *crypto_proto.VeloMessage {
	return self.output
}

func TestResponderWithFlowId(
	config_obj *config_proto.Config, flow_id string) *TestResponderType {
	ctx := context.Background()

	output_chan, drain := NewMessageDrain(ctx)

	sub_ctx, sub_cancel := context.WithCancel(ctx)

	flow_manager := NewFlowManager(ctx, config_obj, "")
	result := &TestResponderType{
		FlowResponder: &FlowResponder{
			ctx:           sub_ctx,
			cancel:        sub_cancel,
			wg:            &sync.WaitGroup{},
			output:        output_chan,
			logErrorRegex: defaultLogErrorRegex,
			status:        &crypto_proto.VeloStatus{},
		},
		Drain: drain,
	}

	result.status.Status = crypto_proto.VeloStatus_PROGRESS
	result.status.FirstActive = uint64(utils.GetTime().Now().UnixNano() / 1000)
	result.wg.Add(1)
	flow_context := flow_manager.FlowContext(
		result.output, &crypto_proto.VeloMessage{SessionId: flow_id})
	result.FlowResponder.flow_context = flow_context
	result.FlowResponder.flow_context.mu.Lock()
	result.FlowResponder.flow_context.responders = append(
		result.FlowResponder.flow_context.responders, result.FlowResponder)
	result.flow_context.mu.Unlock()

	return result
}

type messageDrain struct {
	mu       sync.Mutex
	cancel   func()
	wg       *sync.WaitGroup
	messages []*crypto_proto.VeloMessage

	Id uint64
}

func (self *messageDrain) Messages() []*crypto_proto.VeloMessage {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]*crypto_proto.VeloMessage, 0, len(self.messages))
	for _, i := range self.messages {
		result = append(result, i)
	}

	return result
}

func NewMessageDrain(ctx context.Context) (
	chan *crypto_proto.VeloMessage, *messageDrain) {
	output_chan := make(chan *crypto_proto.VeloMessage, 100)

	sub_ctx, cancel := context.WithCancel(ctx)

	self := &messageDrain{
		cancel: cancel,
		wg:     &sync.WaitGroup{},
		Id:     utils.GetId(),
	}

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()

		for {
			select {
			case <-sub_ctx.Done():
				return

			case row, ok := <-output_chan:
				if !ok {
					return
				}
				self.mu.Lock()
				self.messages = append(self.messages, row)
				self.mu.Unlock()
			}
		}
	}()

	return output_chan, self
}

func (self *messageDrain) WaitForStatsMessage(t *testing.T) []*crypto_proto.VeloMessage {
	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = self.Messages()
		for _, r := range responses {
			if r.FlowStats != nil &&
				len(r.FlowStats.QueryStatus) > 0 &&
				r.FlowStats.QueryStatus[0].UploadedFiles == 1 {
				return true
			}
		}
		return false
	})
	return responses
}

func (self *messageDrain) WaitForCompletion(t *testing.T) []*crypto_proto.VeloMessage {
	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = self.Messages()
		for _, r := range responses {
			if r.FlowStats != nil &&
				r.FlowStats.FlowComplete {
				return true
			}
		}
		return false
	})
	return responses
}

func (self *messageDrain) WaitForEof(t *testing.T) []*crypto_proto.VeloMessage {
	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = self.Messages()
		for _, r := range responses {
			if r.FileBuffer != nil && r.FileBuffer.Eof {
				return true
			}
		}
		return false
	})
	return responses
}

func (self *messageDrain) WaitForMessage(t *testing.T, count int) []*crypto_proto.VeloMessage {
	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second*5, t, func() bool {
		responses = self.Messages()
		return len(responses) >= count
	})
	return responses
}
