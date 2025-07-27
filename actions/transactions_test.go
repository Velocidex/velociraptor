package actions_test

import (
	"os"

	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

func (self *ClientVQLTestSuite) TestTransactions() {
	test_str := "Hello world"

	tmpfile, err := tempfile.TempFile("")
	assert.NoError(self.T(), err)
	tmpfile.Write([]byte(test_str))
	tmpfile.Close()

	defer os.Remove(tmpfile.Name())

	flow_id := "F.TestTransactions"
	client_id := "C.1234"

	resp := responder.TestResponderWithFlowId(self.ConfigObj, flow_id)

	stat := &crypto_proto.VeloStatus{}

	actions.ResumeTransactions(self.Sm.Ctx, self.ConfigObj, resp, stat,
		&crypto_proto.ResumeTransactions{
			FlowId:   flow_id,
			ClientId: client_id,
			Transactions: []*actions_proto.UploadTransaction{{
				Filename: tmpfile.Name(),
				Accessor: "file",
				// Resume upload from byte 2
				StartOffset: 2,
			}},
			QueryStats: []*crypto_proto.VeloStatus{},
		})

	responses := resp.Drain.WaitForCompletion(self.T())
	assert.True(self.T(), len(responses) > 0)

	// Should send back a standard VQLResponse into the special
	// Server.Internal.ResumedUploads psuedo artifact.
	assert.Contains(self.T(), getVQLResponse(responses), "ReplayTime")
	assert.Contains(self.T(), getVQLResponse(responses),
		"Server.Internal.ResumedUploads")

	// The completed transaction is sent to the server with a response
	// field.
	assert.Contains(self.T(), getUploadTransaction(responses),
		`"response":"{`)

	// The response field contains a hash to signify it is complted.
	assert.Contains(self.T(), getUploadTransaction(responses),
		`"sha256\":\"`)

	// We also send a log to the flow to indicate the transactions are
	// resumed.
	assert.Contains(self.T(), getLogs(responses),
		"Resuming uploads: 1 transactions.")

	// The data sent is actually from offset 2 (Hello World)[2:]
	assert.Contains(self.T(), getFileBuffer(responses),
		`Data: 'llo world'`)

	// The file buffer offset should start at offset 2
	assert.Contains(self.T(), getFileBuffer(responses),
		`Offset: 2`)

	// Upload is completed so an EOF is sent
	assert.Contains(self.T(), getFileBuffer(responses),
		`EOF: true`)
}
