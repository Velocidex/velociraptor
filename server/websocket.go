package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// 10Mb is maximum upload size.
	maxMessageSize = 10 * 1024 * 1024
)

var (
	pleaseEnrollError = errors.New("Please Enrol")
	conflictError     = errors.New("Another Client connection exists. " +
		"Only a single instance of the client is " +
		"allowed to connect at the same time.")
	notConnectedError = errors.New("WS Socket is not connected")

	currentWSConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "client_comms_current_ws_connections",
		Help: "Number of currently connected clients using websockets.",
	})
)

var upgrader = websocket.Upgrader{}

func is_ws_connection(r *http.Request) bool {
	_, pres := r.Header["Upgrade"]
	return pres
}

func ws_server_pem(
	config_obj *config_proto.Config,
	server_obj *Server, w http.ResponseWriter,
	req *http.Request) error {

	ws_, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer ws_.Close()

	key := fmt.Sprintf("server_pem->%v", req.RemoteAddr)
	ws := http_comms.NewWS(key, ws_)
	defer ws.Close()

	for {
		// Just read a message and ignore it.
		_, _, err := ws.ReadMessage()
		if err != nil {
			return send_error(ws, err, http.StatusServiceUnavailable)
		}

		err = send_message(ws, []byte(config_obj.Frontend.Certificate))
		if err != nil {
			return send_error(ws, err, http.StatusServiceUnavailable)
		}
	}
}

// This function enters an infinite loop reading messages from the
// client and processing them.
func ws_receive_client_messages(
	config_obj *config_proto.Config,
	server_obj *Server, w http.ResponseWriter,
	req *http.Request) error {

	ws_, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer ws_.Close()

	// We are receiving messages from the endpoint
	key := fmt.Sprintf("<-%v", req.RemoteAddr)
	ws := http_comms.NewWS(key, ws_)
	defer ws.Close()

	ws.SetPongHandler(config_obj)

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	// Start a ping loop to refresh connections
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-utils.GetTime().After(http_comms.PingWait(config_obj)):
				send_ping(ws, config_obj)
			}
		}
	}()

	// Spin forever reading messages from the client and processing
	// them.
	for {
		ws.SetReadLimit(maxMessageSize)
		_ = ws.SetReadDeadline(utils.Now().Add(
			http_comms.PongPeriod(config_obj)))
		_, message, err := ws.ReadMessage()
		if err != nil {
			return send_error(ws, err, http.StatusServiceUnavailable)
		}

		message_info, err := server_obj.Decrypt(ctx, message)
		if err != nil {
			server_obj.Debug("Unable to decrypt body from %v: %+v "+
				"(%v bytes)", req.RemoteAddr, err, len(message))

			receiveDecryptionErrors.Inc()

			// Just plain reject with a 403.
			return send_error(ws, err, http.StatusForbidden)
		}

		message_info.RemoteAddr = utils.RemoteAddr(
			req, config_obj.Frontend.GetProxyHeader())

		server_obj.Debug("Received a WS post of length %v from %v (%v)",
			len(message), message_info.RemoteAddr, message_info.Source)

		if !message_info.Authenticated {
			err := server_obj.ProcessUnauthenticatedMessages(
				req.Context(), config_obj, message_info)
			if err != nil {
				server_obj.Debug("Unable to process (%v)", message_info.Source)
				return send_error(ws, err, http.StatusServiceUnavailable)
			}

			// We need to indicate to the client to start the
			// enrolment process. Since the client can not read
			// anything from us (because we can not encrypt for
			// it), we indicate this by providing it with an HTTP
			// error code.
			enrollmentCounter.Inc()
			server_obj.Debug("Please Enrol (%v)", message_info.Source)
			return send_error(
				ws, pleaseEnrollError, http.StatusNotAcceptable)
		}

		// Process the request with a different context - if the
		// client disconnects quickly the request context will be
		// cancelled and aborted, but we do not want this to
		// interrupt actually processing the message.
		subctx, cancel := utils.WithTimeoutCause(
			context.Background(), 60*time.Second,
			errors.New("Websocket: deadline reached processing message"))

		_, _, err = server_obj.Process(subctx, message_info,
			DoNotDrainRequestsForClient)
		if err != nil {
			// Send the client an error that indicates the request was
			// incorrect but the client should not retry to send the
			// data.
			_ = send_error(ws, err, http.StatusBadRequest)
			cancel()

		} else {
			// Send an ack to the client that we received this
			// message.
			_ = send_error(ws, nil, http.StatusOK)
			cancel()
		}
	}
}

func send_message(ws *http_comms.Conn, message []byte) error {
	msg := crypto.WSErrorMessage{
		HTTPCode: http.StatusOK,
		Data:     message,
	}
	serialized, _ := json.Marshal(msg)
	deadline := utils.Now().Add(writeWait)
	return ws.WriteMessageWithDeadline(websocket.BinaryMessage, serialized, deadline)
}

// Deliver the HTTP code to the remote end so it can be recreated.
func send_error(ws *http_comms.Conn, err error, code int) error {
	msg := crypto.WSErrorMessage{
		HTTPCode: code,
	}
	if err != nil {
		msg.Error = err.Error()
	}

	serialized, _ := json.Marshal(msg)

	deadline := utils.Now().Add(writeWait)
	err = ws.WriteMessageWithDeadline(websocket.BinaryMessage, serialized, deadline)

	// Wait for the message to be sent to the client side
	// time.Sleep(time.Second)

	return err
}

func send_ping(
	ws *http_comms.Conn,
	config_obj *config_proto.Config) error {
	deadline := utils.Now().Add(http_comms.PongPeriod(config_obj))
	return ws.WriteMessageWithDeadline(websocket.PingMessage, nil, deadline)
}

func ws_send_client_messages(
	config_obj *config_proto.Config,
	server_obj *Server, w http.ResponseWriter,
	req *http.Request) error {
	ws_, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer ws_.Close()

	// Sending messages to the client
	key := fmt.Sprintf("->%v", req.RemoteAddr)
	ws := http_comms.NewWS(key, ws_)
	defer ws.Close()

	// Keep track of currently connected clients.
	currentWSConnections.Inc()
	defer currentWSConnections.Dec()

	ws.SetPongHandler(config_obj)

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	for {
		// Read the first message to authenticate the client's connection
		ws.SetReadLimit(maxMessageSize)
		err := ws.SetReadDeadline(utils.Now().Add(http_comms.PongPeriod(config_obj)))
		if err != nil {
			return err
		}

		_, message, err := http_comms.ReadMessageWithCtx(
			ws, ctx, config_obj)
		if err != nil {
			return send_error(ws, err, http.StatusServiceUnavailable)
		}

		message_info, err := server_obj.Decrypt(ctx, message)
		if err != nil {
			server_obj.Debug("Unable to decrypt body from %v: %+v "+
				"(%v bytes)", req.RemoteAddr, err, len(message))

			receiveDecryptionErrors.Inc()

			// Just plain reject with a 403.
			return send_error(ws, err, http.StatusForbidden)
		}

		message_info.RemoteAddr = utils.RemoteAddr(
			req, config_obj.Frontend.GetProxyHeader())

		server_obj.Debug("Received a WS post of length %v from %v (%v)",
			len(message), message_info.RemoteAddr, message_info.Source)

		// Reject unauthenticated messages. This ensures untrusted
		// clients are not allowed to keep connections open.
		if !message_info.Authenticated {
			return send_error(ws, pleaseEnrollError, http.StatusNotAcceptable)
		}

		// Recover the org for this client
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return send_error(ws, err, http.StatusServiceUnavailable)
		}

		org_config_obj, err := org_manager.GetOrgConfig(message_info.OrgId)
		if err != nil {
			return send_error(ws, err, http.StatusServiceUnavailable)
		}

		// If client is not known, make it enrol. This can happen for
		// example, when the client was just deleted, but we still
		// have ciphers cached to it - the client is not known but we
		// can still verify the comms as authenticated. NOTE: this
		// check should be very quick since it is just a lookup in the
		// client info manager's LRU.
		source := message_info.Source
		client_info_manager, err := services.GetClientInfoManager(org_config_obj)
		if err != nil {
			return send_error(ws, err, http.StatusServiceUnavailable)
		}

		_, err = client_info_manager.Get(ctx, source)
		if err != nil {
			journal, err := services.GetJournal(org_config_obj)
			if err != nil {
				return send_error(ws, err, http.StatusServiceUnavailable)
			}

			// This should trigger an enrollment flow.
			err = journal.PushRowsToArtifact(ctx, org_config_obj,
				[]*ordereddict.Dict{
					ordereddict.NewDict().
						Set("ClientId", source)},
				"Server.Internal.Enrollment", source, "")
			if err != nil {
				return send_error(ws, err, http.StatusServiceUnavailable)
			}

			// Do not serve the client until it has fully enrolled.
			return nil
		}

		// Get a notification for this client from the pool -
		// Must be before the Process() call to prevent race.
		notifier, err := services.GetNotifier(org_config_obj)
		if err != nil {
			return send_error(ws, err, http.StatusServiceUnavailable)
		}

		// Check for conflicting clients
		if notifier.IsClientDirectlyConnected(source) {
			// Send a message that there is a client conflict.
			journal, err := services.GetJournal(org_config_obj)
			if err == nil {
				info := ordereddict.NewDict().
					Set("ClientId", source).
					Set("RemoteAddr", message_info.RemoteAddr).
					Set("UserAgent", req.UserAgent())
				journal.PushRowsToArtifactAsync(ctx, org_config_obj,
					info,
					"Server.Internal.ClientConflict")
			}
			return send_error(ws, conflictError, http.StatusConflict)
		}

		// Unfortunately Gorilla Websocket does not cancel the context
		// when the websocket connection is done properly so we need
		// to do some gymnastics here to detect disconnection, and
		// drop any further client messages. (see
		// https://github.com/gorilla/websocket/issues/633)
		go func() {
			defer cancel()
			defer ws.Close()

			for {
				deadline := utils.Now().Add(http_comms.PongPeriod(config_obj))
				_, _, err = ws.NextReaderWithDeadline(deadline)
				if err != nil {
					return
				}
			}
		}()

		for {
			err := send_one_message(ctx, ws, server_obj,
				org_config_obj, message_info)
			if err != nil {
				return err
			}
		}
	}
}

func send_one_message(
	ctx context.Context,
	ws *http_comms.Conn,
	server_obj *Server,
	org_config_obj *config_proto.Config,
	message_info *crypto.MessageInfo) error {

	source := message_info.Source
	notifier, err := services.GetNotifier(org_config_obj)
	if err != nil {
		return send_error(ws, err, http.StatusServiceUnavailable)
	}

	notification, cancel := notifier.ListenForNotification(source)
	defer cancel()

	// Check for any requests outstanding now.
	response, count, err := server_obj.Process(
		ctx, message_info, DrainRequestsForClient)
	if err != nil {
		return send_error(ws, err, http.StatusBadRequest)
	}

	if count > 0 {
		// Send the new messages to the client
		// and finish the request off.
		err := send_message(ws, response)
		if err != nil {
			return send_error(ws, err, http.StatusBadRequest)
		}
	}

	// Nothing waiting for the client - wait here for new
	// notification.
	select {
	case <-ctx.Done():
		return notConnectedError

	case <-utils.GetTime().After(http_comms.PingWait(org_config_obj)):
		return send_ping(ws, org_config_obj)

	case <-notification:
		response, count, err := server_obj.Process(
			ctx, message_info, DrainRequestsForClient)
		if err != nil {
			return send_error(ws, err, http.StatusBadRequest)
		}

		if count == 0 {
			return nil
		}

		// Send the new messages to the client
		err = send_message(ws, response)
		if err != nil {
			return send_error(ws, err, http.StatusBadRequest)
		}
	}

	return nil
}
