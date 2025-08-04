package vtesting

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
)

// Wrap httptest.NewServer to make it listen on a fixed port.

func NewServer(app http.Handler, port int) *httptest.Server {
	ts := httptest.NewUnstartedServer(app)
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		panic(fmt.Sprintf("httptest: failed to listen on %v: %v", port, err))
	}
	ts.Listener = l
	ts.Start()
	return ts
}
