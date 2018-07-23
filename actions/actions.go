// Client actions are routines that run on the client and return a
// GrrMessage.
package actions

import (
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type ClientAction interface {
	Run(ctx *context.Context,
		args *crypto_proto.GrrMessage,
		output chan<- *crypto_proto.GrrMessage)
}

func GetClientActionsMap() map[string]ClientAction {
	result := make(map[string]ClientAction)
	result["GetClientInfo"] = &GetClientInfo{}
	result["StatFile"] = &StatFile{}
	result["ListDirectory"] = &ListDirectory{}
	result["HashFile"] = &HashFile{}
	result["HashBuffer"] = &HashBuffer{}
	result["TransferBuffer"] = &TransferBuffer{}
	result["VQLClientAction"] = &VQLClientAction{}
	result["GetHostname"] = &GetHostname{}
	result["GetPlatformInfo"] = &GetPlatformInfo{}
	result["UpdateForeman"] = &UpdateForeman{}

	return result
}
