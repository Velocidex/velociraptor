package responder

import (
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

func MakeErrorResponse(
	output chan *crypto_proto.VeloMessage, session_id string, message string) {
	output <- &crypto_proto.VeloMessage{
		SessionId: session_id,
		RequestId: constants.LOG_SINK,
		LogMessage: &crypto_proto.LogMessage{
			NumberOfRows: 1,
			Jsonl: json.Format(
				"{\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
				int(utils.GetTime().Now().Unix()), logging.ERROR, message),
			ErrorMessage: message,
		},
	}
}
