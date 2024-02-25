package crypto

// Our client side handler emulates a direct HTTP connection over
// websockets. Therefore the server needs to embody the same
// parameters that allow the client to recreate the relevant
// http.Response object.
type WSErrorMessage struct {
	HTTPCode int    `json:"code,omitempty"`
	Error    string `json:"err,omitempty"`
	Data     []byte `json:"data,omitempty"`
}
