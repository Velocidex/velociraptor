package utils

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// https://github.com/grpc/grpc-go/issues/1229#issuecomment-302755717
// DoWithTimeout runs f and returns its error.  If the deadline d
// elapses first, it returns a grpc DeadlineExceeded error instead.
func DoWithTimeout(f func() error, d time.Duration) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- f()
		close(errChan)
	}()
	t := time.NewTimer(d)
	select {
	case <-t.C:
		return status.Errorf(codes.DeadlineExceeded, "too slow")
	case err := <-errChan:
		if !t.Stop() {
			<-t.C
		}
		return err
	}
}
