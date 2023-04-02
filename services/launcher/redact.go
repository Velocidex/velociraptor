package launcher

import (
	"google.golang.org/protobuf/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

func redactTask(task *crypto_proto.VeloMessage) *crypto_proto.VeloMessage {
	result := proto.Clone(task).(*crypto_proto.VeloMessage)
	if result.FlowRequest != nil {
		for _, query := range result.FlowRequest.VQLClientActions {
			for _, parameter := range query.Env {
				if parameter.Comment == "redacted" {
					parameter.Value = "XXXXX"
				}
			}
		}
	}

	return result
}

func redactCollectContext(
	collection_context *flows_proto.ArtifactCollectorContext) *flows_proto.ArtifactCollectorContext {
	result := proto.Clone(collection_context).(*flows_proto.ArtifactCollectorContext)
	if result.Request != nil {
		for _, spec := range result.Request.Specs {
			if spec.Parameters == nil {
				continue
			}
			for _, p := range spec.Parameters.Env {
				if p.Comment == "redacted" {
					p.Value = "XXXXX"
				}
			}
		}
	}
	return result
}
