/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
// Velociraptor flows are similar to GRR's flows but are optimized for
// speed:

// 1. Velociraptor flows are run directly on the frontend - there is
//    no secondary queuing to a worker like in GRR. This means that
//    flows need to be efficient and ideally simply store their
//    results in the database.

// 2. GRR flows are a state machine, where each CallClient() request
//    may solicit multiple responses. GRR queues these responses
//    before calling the state method so they are delivered to the
//    flow in order. This is inefficient and requires a lot of
//    processing on the front ends. In contrast Velociraptor flows are
//    not state machines - they receive the response to each request
//    as soon as it arrives (without waiting for a status
//    message). Therefore responses may be processed out of order:

// Consider a request issued to the client will generate 2
// responses. The client may send response 1 in one POST operation and
// response 2 in another POST operation.

// Server sends:
//  - request id 1

// Client Post 1:
//  - request id 1, respone id 1

// Client Post 2:
//  - respone id 1, respone id 2
//  - respone id 1, respone id 3, status OK.

// Velociraptor will a actually run the handler three times - for each
// response and for the status.

// GRR flows maintain state while processing each response. The server
// will load the state from the database and prepare it for the worker
// to process. In contrast, Velociraptor does not maintain state
// between invocations. If the flow needs to keep state, they should
// mainain that themselves.

// GRR was designed to allow flows to be long lived, process long
// request/response sequences. In Velociraptor, the focus has shifted
// to make flows very simple and maintain little state. Generally
// flows simply issue client requests and receive their responses to
// store in the database.

package flows
