package utils

import (
	"errors"
	"fmt"
	"os"
)

var (
	// Error relayed when the error details are added inline. The GUI
	// API will strip this error as the details are included in the
	// response already.
	InlineError         = errors.New("InlineError")
	TimeoutError        = errors.New("Timeout")
	InvalidStatus       = errors.New("InvalidStatus")
	TypeError           = errors.New("TypeError")
	NotImplementedError = errors.New("Not implemented")
	InvalidConfigError  = errors.New("InvalidConfigError")
	NotFoundError       = Wrap(os.ErrNotExist, "NotFoundError")
	InvalidArgError     = errors.New("InvalidArgError")
	IOError             = errors.New("IOError")
	NoAccessToOrgError  = errors.New("No access to org")
	CancelledError      = errors.New("Cancelled")
	SecretsEnforced     = errors.New("Secrets are enforced - you must specify a secret name")
	PermissionDenied    = errors.New("PermissionDenied")
)

// This is a custom error type that wraps an inner error but does not
// propegate its output. It is similar to fmt.Errorf() except does not
// print the underlying error string.

// This is useful in order to hide the specific error message from the
// implementation but retain its type. For example, wrapping an
// os.ErrNotExist from the filesystem to represent a non existant flow
// or hunt. Callers can then check for a standard error using
// errors.Is(err, os.ErrNotExist) but the actual error message is
// suppressed.
type Error struct {
	Inner  error
	Format string
	Args   []interface{}
}

func (self *Error) Error() string {
	return fmt.Sprintf(self.Format, self.Args...)
}

func (e *Error) Unwrap() error {
	return e.Inner
}

func Wrap(err error, format string, args ...interface{}) error {
	return &Error{
		Inner:  err,
		Format: format,
		Args:   args,
	}
}

// Format an error as a string.
func Errf(e error) interface{} {
	if e == nil {
		return nil
	}
	return e.Error()
}

func IsNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
