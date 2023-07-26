package utils

import errors "github.com/go-errors/errors"

var (
	// Error relayed when the error details are added inline. The GUI
	// API will strip this error as the details are included in the
	// response already.
	InlineError = errors.New("InlineError")
)
