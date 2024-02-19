package http

import "net/http"

// Record the status of the request so we can log it.
type StatusRecorder struct {
	http.ResponseWriter
	http.Flusher
	Status int
	Error  []byte
}

func (self *StatusRecorder) WriteHeader(code int) {
	self.Status = code
	self.ResponseWriter.WriteHeader(code)
}

func (self *StatusRecorder) Write(buf []byte) (int, error) {
	if self.Status == 500 {
		self.Error = buf
	}

	return self.ResponseWriter.Write(buf)
}
