package flows

import (
	"google.golang.org/protobuf/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	utils "www.velocidex.com/golang/velociraptor/utils"
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

// The collection_context contains high level stats that summarise the
// colletion. We derive this information from the specific results of
// each query.
func UpdateFlowStats(collection_context *CollectionContext) {
	// Support older colletions which do not have this info
	if len(collection_context.QueryStats) == 0 {
		return
	}

	// Now update the overall collection statuses based on all the
	// individual query status. The collection status is a high level
	// overview of the entire collection.
	collection_context.State = flows_proto.ArtifactCollectorContext_RUNNING
	collection_context.Status = ""
	collection_context.Backtrace = ""
	for _, s := range collection_context.QueryStats {
		// Get the first errored query.
		if collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING &&
			s.Status != crypto_proto.VeloStatus_OK {
			collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
			collection_context.Status = s.ErrorMessage
			collection_context.Backtrace = s.Backtrace
			break
		}
	}

	// Total execution duration is the sum of all the query durations
	// (this can be faster than wall time if queries run in parallel)
	collection_context.ExecutionDuration = 0
	for _, s := range collection_context.QueryStats {
		collection_context.ExecutionDuration += s.Duration

		for _, a := range s.NamesWithResponse {
			if !utils.InString(collection_context.ArtifactsWithResults, a) {
				collection_context.ArtifactsWithResults = append(
					collection_context.ArtifactsWithResults, a)
			}
		}
	}

	collection_context.OutstandingRequests = collection_context.TotalRequests -
		int64(len(collection_context.QueryStats))
	if collection_context.OutstandingRequests <= 0 &&
		collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING {
		collection_context.State = flows_proto.ArtifactCollectorContext_FINISHED
	}
}
