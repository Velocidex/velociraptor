package json

import (
	"sync"

	"github.com/Velocidex/json"
)

var (
	mu       sync.Mutex
	handlers = []*encoderHandler{}
)

type encoderHandler struct {
	sample interface{}
	cb     json.EncoderCallback
}

// Callers can register their custom encoders through this
// function. Should be done once from an init() function.
func RegisterCustomEncoder(sample interface{}, cb json.EncoderCallback) {
	mu.Lock()
	defer mu.Unlock()

	handlers = append(handlers, &encoderHandler{sample, cb})
}

func NewEncOpts() *json.EncOpts {
	mu.Lock()
	defer mu.Unlock()

	opts := json.NewEncOpts()
	for _, h := range handlers {
		opts.WithCallback(h.sample, h.cb)
	}
	return opts
}
