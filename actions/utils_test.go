package actions_test

import (
	"fmt"

	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

// Format various response packets so they can be better matched for
// tests.
func getLogs(responses []*crypto_proto.VeloMessage) string {
	result := ""
	for _, item := range responses {
		if item.LogMessage != nil {
			result += item.LogMessage.Jsonl + "\n"
		}
	}

	return result
}

func getUploadTransaction(responses []*crypto_proto.VeloMessage) string {
	result := ""
	for _, item := range responses {
		if item.UploadTransaction != nil {
			result += json.MustMarshalString(item.UploadTransaction)
		}
	}

	return result
}

func getFileBuffer(responses []*crypto_proto.VeloMessage) string {
	result := ""
	for _, item := range responses {
		if item.FileBuffer != nil {
			result += fmt.Sprintf(
				"Offset: %v, Data: '%v' Data Length: %v EOF: %v\n",
				item.FileBuffer.Offset,
				string(item.FileBuffer.Data),
				item.FileBuffer.DataLength,
				item.FileBuffer.Eof)
		}
	}

	return result
}

func getVQLResponse(responses []*crypto_proto.VeloMessage) string {
	for _, item := range responses {
		if item.VQLResponse != nil {
			return fmt.Sprintf("Target: %v, JSONL: %v\n",
				item.VQLResponse.Query.Name,
				item.VQLResponse.JSONLResponse)
		}
	}

	return ""
}
