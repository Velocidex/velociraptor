package logging

// Logging levels for responder.Log()
const (
	DEFAULT = "DEFAULT"
	ERROR   = "ERROR"
	INFO    = "INFO"
	WARNING = "WARN"
	DEBUG   = "DEBUG"
	WARN    = "WARN"

	// An alert is a special type of log message which is routed by
	// the server into the alert queue.
	ALERT = "ALERT"
)
