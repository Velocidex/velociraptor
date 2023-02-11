/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
package server

import (
	"bytes"
	"context"
	"errors"
	"html"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"

	"github.com/Velocidex/ordereddict"
	"github.com/juju/ratelimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	packetTooLargeError = errors.New("Packet too large!")

	currentConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "client_comms_current_connections",
		Help: "Number of currently connected clients.",
	})

	redirectedFrontendCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_redirect_count",
		Help: "Number of times the frontend redirected.",
	})

	sendCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_send_count",
		Help: "Number of POST requests frontend sent to the client.",
	})

	// Normally this is calculated in Graphana but it is also
	// convenient to have an approximation right here.
	receiveQPS = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "frontend_receive_QPS",
		Help: "QPS of receive handler.",
	})

	receiveCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_received_count",
		Help: "Number of POST requests frontend received from the client.",
	})

	receiveBytesCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_received_bytes",
		Help: "Number of bytes received from the client.",
	})

	receiveDecryptionErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_decryption_errors",
		Help: "Number of errors in decrypting messages.",
	})

	enrollmentCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_enroll_response",
		Help: "Number responses to enrol (406).",
	})

	urgentCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_urgent_responses",
		Help: "Number urgent responses received.",
	})

	timeoutCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_timeout_rejections",
		Help: "Number of responses rejected due to concurrency timeouts.",
	})

	concurrencyHistorgram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "frontend_receiver_latency",
			Help:    "Latency to receive client data in second.",
			Buckets: prometheus.LinearBuckets(0.1, 1, 10),
		},
	)

	concurrencyWaitHistorgram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "frontend_concurrency_wait_latency",
			Help:    "Latency for clients waiting to get a concurrency slot (excludes actual serving time).",
			Buckets: prometheus.LinearBuckets(0.1, 1, 10),
		},
	)
)

func PrepareFrontendMux(
	config_obj *config_proto.Config,
	server_obj *Server,
	router *http.ServeMux) error {

	if config_obj.Frontend == nil {
		return errors.New("Frontend not configured")
	}

	base := config_obj.Frontend.BasePath
	router.Handle(base+"/healthz", healthz(server_obj))
	router.Handle(base+"/server.pem", server_pem(config_obj))

	// DEPRECATED: These are the old handler names - not great
	// but here for backwards compatibility.
	router.Handle(base+"/control",
		RecordHTTPStats(receive_client_messages(config_obj, server_obj)))
	router.Handle(base+"/reader",
		RecordHTTPStats(send_client_messages(server_obj)))

	// Send a message to the server.
	router.Handle(base+"/send_messages",
		RecordHTTPStats(receive_client_messages(config_obj, server_obj)))

	// Receive new messages from the server.
	router.Handle(base+"/receive_messages",
		RecordHTTPStats(send_client_messages(server_obj)))

	// Publicly accessible part of the filestore. NOTE: this
	// does not have to be a physical directory - it is served
	// from the filestore.
	router.Handle(base+"/public/", GetLoggingHandler(config_obj, "/public")(
		http.StripPrefix(base,
			downloadPublic(config_obj, []string{"public"}))))

	return nil
}

func healthz(server_obj *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&server_obj.Healthy) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

func server_pem(config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		flusher.Flush()

		_, _ = w.Write([]byte(config_obj.Frontend.Certificate))
	})
}

// Redirect client to another active frontend.
/* Experimental code disabled for now.
func maybeRedirectFrontend(handler string, w http.ResponseWriter, r *http.Request) bool {
	_, pres := r.URL.Query()["r"]
	if pres {
		return false
	}

	redirect_url, ok := services.Frontend.GetFrontendURL()
	if ok {
		redirectedFrontendCounter.Inc()
		// We should redirect to another frontend.
		http.Redirect(w, r, redirect_url, 301)
		return true
	}

	// Handle request ourselves.
	return false
}
*/

// Read the message from the client carefully. Due to concurrency
// control we want to dismiss slow clients as soon as possible since
// processing them is taking a concurrency slot and causes slow down
// for other clients.  We read the message and decrypt it before
// taking the concurrency slot up.
func readWithLimits(
	ctx context.Context,
	config_obj *config_proto.Config,
	server_obj *Server,
	req *http.Request) (*crypto.MessageInfo, error) {

	// Read the data from the POST request into a
	buffer := &bytes.Buffer{}
	max_upload_size := config_obj.Frontend.Resources.MaxUploadSize
	if max_upload_size == 0 {
		max_upload_size = 5000000
	}

	reader := io.LimitReader(req.Body, int64(max_upload_size*2))

	// Implement rate limiting from reading the connection.
	if config_obj.Frontend.Resources.PerClientUploadRate > 0 {
		bucket := ratelimit.NewBucketWithRate(
			float64(config_obj.Frontend.Resources.PerClientUploadRate),
			100*1024)
		reader = ratelimit.Reader(reader, bucket)
	}

	if server_obj.Bucket != nil {
		reader = ratelimit.Reader(reader, server_obj.Bucket)
	}

	n, err := utils.Copy(ctx, buffer, reader)
	if err != nil {
		return nil, err
	}
	receiveBytesCounter.Add(float64(n))

	if uint64(n) >= max_upload_size*2 {
		server_obj.Error("Size exceeded when reading body from %v",
			req.RemoteAddr)

		return nil, packetTooLargeError
	}

	message_info, err := server_obj.Decrypt(ctx, buffer.Bytes())
	if err != nil {
		server_obj.Debug("Unable to decrypt body from %v: %+v "+
			"(%v out of max %v)", req.RemoteAddr, err, n, max_upload_size*2)

		receiveDecryptionErrors.Inc()
		return nil, errors.New("Unable to decrypt")
	}
	message_info.RemoteAddr = utils.RemoteAddr(req, config_obj.Frontend.GetProxyHeader())
	server_obj.Debug("Received a post of length %v from %v (%v)",
		n, message_info.RemoteAddr, message_info.Source)

	return message_info, nil
}

// This handler is used to receive messages from the client to the
// server. These connections are short lived - the client will just
// post its message and then disconnect.
func receive_client_messages(
	config_obj *config_proto.Config, server_obj *Server) http.Handler {
	pad := &crypto_proto.ClientCommunication{}
	pad.Padding = append(pad.Padding, 0)
	serialized_pad, _ := proto.Marshal(pad)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("http handler is not a flusher")
		}

		// Allow a limited time to read from the client because this
		// is the hot path.
		ctx, cancel := context.WithTimeout(req.Context(), 600*time.Second)
		defer cancel()

		// If the client connection drops we close the reader.
		notify, ok := w.(http.CloseNotifier)
		if ok {
			notifier := notify.CloseNotify()
			go func() {
				select {
				case <-notifier:
					cancel()

				case <-ctx.Done():
					return
				}
			}()
		}

		receiveCounter.Inc()

		priority := req.Header.Get("X-Priority")
		// For urgent messages skip concurrency control - This
		// allows clients with urgent messages to always be
		// processing even when the frontend are loaded.
		if priority != "urgent" {
			// Keep track of the average time the request spends
			// waiting for a concurrency slot. If this time is too
			// long it means concurrency may need to be increased.
			timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
				concurrencyWaitHistorgram.Observe(v)
			}))

			cancel, err := server_obj.Concurrency().StartConcurrencyControl(ctx)
			if err != nil {
				http.Error(w, "Timeout", http.StatusRequestTimeout)
				timeoutCounter.Inc()
				return
			}
			defer cancel()

			timer.ObserveDuration()

		} else {
			urgentCounter.Inc()
		}

		// Measure the latency from this point on.
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			concurrencyHistorgram.Observe(v)
		}))
		defer func() {
			timer.ObserveDuration()
		}()

		// Read the payload from the client.
		message_info, err := readWithLimits(ctx, config_obj, server_obj, req)
		if err != nil {
			// Drop the packet on the floor to release it from the
			// client's queue. If the client sends a very large packet
			// it will be truncated by the above limit, and will not
			// be possible to decrypt it. By dropping it on the floor
			// we release it from the client's queue - otherwise it
			// will just retransmit the same thing again and the
			// packet will be stuck.
			if err == packetTooLargeError {
				w.WriteHeader(http.StatusOK)
				flusher.Flush()
				return
			}

			// Just plain reject with a 403.
			http.Error(w, "", http.StatusForbidden)
			return
		}

		// Very few Unauthenticated client messages are valid
		// - currently only enrolment requests.
		if !message_info.Authenticated {
			err := server_obj.ProcessUnauthenticatedMessages(
				req.Context(), config_obj, message_info)
			if err == nil {
				// We need to indicate to the client
				// to start the enrolment
				// process. Since the client can not
				// read anything from us (because we
				// can not encrypt for it), we
				// indicate this by providing it with
				// an HTTP error code.
				enrollmentCounter.Inc()
				server_obj.Debug("Please Enrol (%v)", message_info.Source)
				http.Error(
					w,
					"Please Enrol",
					http.StatusNotAcceptable)
			} else {
				server_obj.Debug("Unable to process (%v)", message_info.Source)
				http.Error(w, "", http.StatusServiceUnavailable)
			}
			return
		}

		// From here below we have received the client payload
		// and it should not resend it to us. We need to keep
		// the client blocked until we finish processing the
		// flow as a method of rate limiting the clients. We
		// do this by streaming pad packets to the client,
		// while the flow is processed.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		sync := make(chan []byte)
		go func() {
			defer close(sync)

			// Process the request with a different context - if the
			// client disconnects quickly the request context will be
			// cancelled and aborted, but we do not want this to
			// interrupt actually processing the message.
			subctx, cancel := context.WithTimeout(context.Background(),
				60*time.Second)
			defer cancel()

			response, _, err := server_obj.Process(subctx, message_info,
				false, // drain_requests_for_client
			)
			if err != nil {
				server_obj.Error("Error: %v", err)
				return
			}

			// Wait here for the code below to read from the sync
			// channel so they can send the results back. If the
			// client disconnected and the code below has exited we
			// block here for up to 3 seconds before cancelling the
			// request anyway (and not sending reply to the client).
			select {
			case <-subctx.Done():
			case sync <- response:
			case <-time.After(3 * time.Second):
			}
		}()

		// Spin here on the response being ready, every few
		// seconds we send a pad packet to the client to keep
		// the connection active. The pad packet is sent using
		// chunked encoding and will be assembled by the
		// client into the complete protobuf when it is
		// read. Since protobuf serialization does not have
		// headers, it is safe to just concat multiple binary
		// encodings into a single proto. This means the
		// decoded protobuf will actually contain the full
		// response as well as the padding fields (which are
		// ignored).
		for {
			select {
			case <-ctx.Done():
				return

			case response, ok := <-sync:
				if ok {
					_, _ = w.Write(response)
				}
				return

			case <-time.After(3 * time.Second):
				_, _ = w.Write(serialized_pad)
				flusher.Flush()
			}
		}
	})
}

// This handler is used to send messages to the client. This
// connection will persist up to Client.MaxPoll so we always have a
// channel to the client. This allows us to send the client jobs
// immediately with low latency.
func send_client_messages(server_obj *Server) http.Handler {
	pad := &crypto_proto.ClientCommunication{}
	pad.Padding = append(pad.Padding, 0)
	serialized_pad, _ := proto.Marshal(pad)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("http handler is not a flusher")
		}

		sendCounter.Inc()

		// Keep track of currently connected clients.
		currentConnections.Inc()
		defer currentConnections.Dec()

		body, err := ioutil.ReadAll(
			io.LimitReader(req.Body, constants.MAX_MEMORY))
		if err != nil {
			server_obj.Error("Unable to read body: %v", err)
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}

		message_info, err := server_obj.Decrypt(req.Context(), body)
		if err != nil {
			// Just plain reject with a 403.
			http.Error(w, "", http.StatusForbidden)
			return
		}
		message_info.RemoteAddr = req.RemoteAddr

		// Reject unauthenticated messages. This ensures
		// untrusted clients are not allowed to keep
		// connections open.
		if !message_info.Authenticated {
			http.Error(w, "Please Enrol", http.StatusNotAcceptable)
			return
		}

		// Recover the org for this client
		org_manager, err := services.GetOrgManager()
		if err != nil {
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}

		org_config_obj, err := org_manager.GetOrgConfig(message_info.OrgId)
		if err != nil {
			server_obj.Info("reader: Unknown org ID %v", message_info.OrgId)
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}

		// Get a notification for this client from the pool -
		// Must be before the Process() call to prevent race.
		source := message_info.Source

		client_info_manager, err := services.GetClientInfoManager(org_config_obj)
		if err != nil {
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}

		// If client is not known, make it enrol. This can happen for
		// example, when the client was just deleted, but we still
		// have ciphers cached to it - the client is not known but we
		// can still verify the comms as authenticated. NOTE: this
		// check should be very quick since it is just a lookup in the
		// client info manager's LRU.
		_, err = client_info_manager.Get(ctx, source)
		if err != nil {
			journal, err := services.GetJournal(org_config_obj)
			if err != nil {
				http.Error(w, "", http.StatusServiceUnavailable)
				return
			}

			// This should trigger an enrollment flow.
			err = journal.PushRowsToArtifact(ctx, org_config_obj,
				[]*ordereddict.Dict{
					ordereddict.NewDict().
						Set("ClientId", source)},
				"Server.Internal.Enrollment", source, "")
			if err != nil {
				http.Error(w, "", http.StatusServiceUnavailable)
				return
			}

			// Do not serve the client until it has fully enrolled.
			return
		}

		notifier, err := services.GetNotifier(org_config_obj)
		if err != nil {
			http.Error(w, "Shutting down", http.StatusServiceUnavailable)
			return
		}

		// Check for conflicting clients
		if notifier.IsClientDirectlyConnected(source) {

			// Send a message that there is a client conflict.
			journal, err := services.GetJournal(org_config_obj)
			if err == nil {
				journal.PushRowsToArtifactAsync(ctx, org_config_obj,
					ordereddict.NewDict().Set("ClientId", source),
					"Server.Internal.ClientConflict")
			}

			http.Error(w, "Another Client connection exists. "+
				"Only a single instance of the client is "+
				"allowed to connect at the same time.",
				http.StatusConflict)
			return
		}

		notification, cancel := notifier.ListenForNotification(source)
		defer cancel()

		// Deadlines are designed to ensure that connections
		// are not blocked for too long (maybe several
		// minutes). This helps to expire connections when the
		// client drops off completely or is behind a proxy
		// which caches the heartbeats. After the deadline we
		// close the connection and expect the client to
		// reconnect again. We add a bit of jitter to ensure
		// clients do not get synchronized.
		wait := time.Duration(org_config_obj.Client.MaxPoll+
			uint64(rand.Intn(30))) * time.Second

		deadline := time.After(wait)

		// We now write the header and block the client until
		// a notification is sent on the notification pool.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		// Check for any requests outstanding now.
		response, count, err := server_obj.Process(
			req.Context(), message_info,
			true, // drain_requests_for_client
		)
		if err != nil {
			server_obj.Error("Error: %v", err)
			return
		}

		if count > 0 {
			// Send the new messages to the client
			// and finish the request off.
			n, err := w.Write(response)
			if err != nil || n < len(serialized_pad) {
				server_obj.Info("reader: Error %v", err)
			}
			return
		}

		// Nothing waiting for the client - wait here for new
		// notification.
		for {
			select {
			// Figure out when the client drops the
			// connection so we can exit.
			case <-ctx.Done():
				return

			case quit := <-notification:
				if quit {
					server_obj.Debug("reader: quit.")
					return
				}
				response, _, err := server_obj.Process(
					req.Context(),
					message_info,
					true, // drain_requests_for_client
				)
				if err != nil {
					server_obj.Error("Error: %v", err)
					return
				}

				// Send the new messages to the client
				// and finish the request off.
				n, err := w.Write(response)
				if err != nil || n < len(serialized_pad) {
					server_obj.Debug("reader: Error %v", err)
				}

				flusher.Flush()
				return

			case <-deadline:
				// Deadline exceeded - write an empty response and
				// send it. The client will reconnect immediately.
				_, err := w.Write(serialized_pad)
				if err != nil {
					server_obj.Debug("reader: Error %v", err)
					return
				}

				flusher.Flush()
				return

				// Write a pad message every 10 seconds
				// to keep the conenction alive.
			case <-time.After(10 * time.Second):
				_, err := w.Write(serialized_pad)
				if err != nil {
					server_obj.Debug("reader: Error %v", err)
					return
				}

				flusher.Flush()
			}
		}
	})
}

// Record the status of the request so we can log it.
type statusRecorder struct {
	http.ResponseWriter
	http.Flusher
	status int
	error  []byte
}

func (self *statusRecorder) WriteHeader(code int) {
	self.status = code
	self.ResponseWriter.WriteHeader(code)
}

func (self *statusRecorder) Write(buf []byte) (int, error) {
	if self.status == 500 {
		self.error = buf
	}

	return self.ResponseWriter.Write(buf)
}

func GetLoggingHandler(config_obj *config_proto.Config,
	handler string) func(http.Handler) http.Handler {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{
				w,
				w.(http.Flusher),
				200, nil}

			defer func() {
				logger.WithFields(
					logrus.Fields{
						"method":     r.Method,
						"url":        r.URL.Path,
						"remote":     r.RemoteAddr,
						"user-agent": r.UserAgent(),
						"status":     rec.status,
						"handler":    handler,
					}).Info("Access to handler")
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

func downloadPublic(
	config_obj *config_proto.Config, prefix []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path_spec := paths.FSPathSpecFromClientPath(r.URL.Path)
		components := path_spec.Components()

		// make sure the prefix is correct
		for i, p := range prefix {
			if len(components) <= i || p != components[i] {
				returnError(w, 404, "Not Found")
				return
			}
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		fd, err := file_store_factory.ReadFile(path_spec)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		// From here on we already sent the headers and we can
		// not really report an error to the client.
		w.Header().Set("Content-Disposition", "attachment; filename="+
			url.PathEscape(path_spec.Base())+api.GetExtensionForFilestore(path_spec))

		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

		utils.Copy(r.Context(), w, fd)
	})
}

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	_, _ = w.Write([]byte(html.EscapeString(message)))
}

// Calculate QPS
func init() {
	utils.RegisterQPSCounter(receiveCounter, receiveQPS)
}
