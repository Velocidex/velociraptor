package hunt_dispatcher

// The hunt dispatcher is a local in memory cache of current active
// hunts. As clients check in to the frontend, the server makes sure
// there are no outstanding hunts for that client, and this needs to
// be in memory for quick access. The hunt dispatcher refreshes the
// hunt list periodically from the data store to receive fresh data.

// In multi frontend deployments, each node (master or minion) has its
// own hunt dispatcher, initialized from the data store. On minion
// nodes, the hunt dispatcher is not allowed to write updates to the
// data store, only read them.

// The master's hunt dispatcher is responsible for maintaining the
// hunt state across all nodes. In order to update a hunt's property
// (e.g. TotalClientsScheduled etc), callers should call MutateHunt()
// on their local node to send a mutation to the master, which will
// actually update the hunt state.

// As the hunt manager (singleton running on the master) updates the
// hunt record, it sends the new record to the
// Server.Internal.HuntUpdate queue, where all hunt dispatchers will
// receive it and update their internal state. The hunt dispatcher on
// the master will also write the new record to the data store.

// NOTE: The master's hunt dispatcher is the only component that has
// write access to the datastore. Updates are received through
// mutations on the Server.Internal.HuntUpdate queue.

// Hunt Dispatcher:
// 1. Maintain an in memory list of hunts.
// 2. Master only:
//    * Mediate access to hunt storage that reflects the memory version.
//    * Maintain hunt index - a fast index for loading hunt objects.
// 3. Minion only:
//    * Read hunt index periodically to refresh in memory hunts cache.

// Listening Queues:
// 1. Server.Internal.HuntUpdate:
//   * Receives mutation to update the hunt object in memory. Should
//     keep in memory in sync with master but periodic refresh anyway
//     to ensure consistency.

// Sending events:
// 1. System.Hunt.Archive:
//    * Allow artifacts to trigger on when hunts are being archived.
// 2. System.Hunt.Participation
// 3. Server.Internal.HuntModification
// 4. Server.Internal.HuntUpdate
// 5. System.Hunt.Creation

// Hunt Manager:

// Listening Queues:
// 1. Server.Internal.HuntModification:
//    * Receives mutation to update the hunt. Mutations include,
//      start/stop, modify description, tags etc.
// 2. System.Hunt.Participation:
//    * Receives message from foreman about possible hunt participation
//    * considers the client and may add to the hunt.
// 3. Server.Internal.Label:
//    * When a label is added to a client, check if a hunt must be scheduled on it.
// 4. Server.Internal.Interrogation
//    * When a client is interrogated, check if a hunt must be scheduled on it.
// 5. System.Flow.Completion:
//    * When a flow is complete - increment hunt stats

// Sending events:
// 1. System.Hunt.Participation:
//   * Used to schedule a client on a hunt.
