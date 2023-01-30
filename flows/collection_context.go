// NOTE: This file implements the old Client communication protocol. It is
// here to provide backwards communication with older clients and will
// eventually be removed.

package flows

import (
	"google.golang.org/protobuf/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

func deobfuscateNames(config_obj *config_proto.Config,
	names []string) []string {
	deobfuscated_names := make([]string, 0, len(names))
	for _, n := range names {
		deobfuscated_names = append(deobfuscated_names,
			artifacts.DeobfuscateString(config_obj, n))
	}

	return deobfuscated_names
}

// Track stats for each query independently - then combine the whole
// thing into the overall collection stats.
func updateQueryStats(
	config_obj *config_proto.Config,
	collection_context *CollectionContext,
	status *crypto_proto.VeloStatus) {

	if len(status.NamesWithResponse) > 0 {
		status.NamesWithResponse = deobfuscateNames(config_obj, status.NamesWithResponse)
	}

	for idx, existing_stat := range collection_context.QueryStats {
		if status.QueryId == existing_stat.QueryId {
			// Error statuses are sticky - the first error stats for a
			// query id will stick to the status and remove previous
			// OK status. Further error status will be ignored.
			if status.Status != crypto_proto.VeloStatus_OK &&
				existing_stat.Status == crypto_proto.VeloStatus_OK {
				new_stat := proto.Clone(status).(*crypto_proto.VeloStatus)
				new_stat.Duration += status.Duration
				new_stat.NamesWithResponse = append(new_stat.NamesWithResponse,
					existing_stat.NamesWithResponse...)
				collection_context.QueryStats[idx] = new_stat
				continue
			}

			if status.Artifact != "" {
				existing_stat.Artifact = status.Artifact
			}

			if status.Duration > existing_stat.Duration {
				existing_stat.Duration = status.Duration
			}

			if len(status.NamesWithResponse) > len(existing_stat.NamesWithResponse) {
				existing_stat.NamesWithResponse = status.NamesWithResponse
			}

			// On older versions of the client QueryId is not
			// propagated properly so we end up with all statuses with
			// a query id of 0. In this case we should keep all the
			// statuses even if they are already received so we can
			// terminate the flow.
			status.Duration = 0
			collection_context.QueryStats = append(collection_context.QueryStats, status)
			return
		}
	}

	// We dont have this status yet
	collection_context.QueryStats = append(collection_context.QueryStats, status)
}
