package assert

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func Retry(t *testing.T, maxAttempts int, sleep time.Duration, f func(r *R)) bool {
	error_log := &bytes.Buffer{}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		r := &R{
			MaxAttempts: maxAttempts,
			Attempt:     attempt,
			log:         error_log,
		}

		f(r)

		if !r.failed {
			// Report all previous failures for the logs so we can
			// identify the flakey tests
			if attempt > 0 {
				t.Logf("Success after %d attempts:%s", attempt, r.log.String())
			}
			return true
		}

		if attempt == maxAttempts {
			t.Logf("FAILED after %d attempts:%s", attempt, r.log.String())
			t.Fail()
		}

		time.Sleep(sleep)
	}
	return false
}

// R is passed to each run of a flaky test run, manages state and accumulates log statements.
type R struct {
	MaxAttempts int
	Attempt     int

	failed bool
	log    *bytes.Buffer
}

// Fail marks the run as failed, and will retry once the function returns.
func (r *R) Fail() {
	r.failed = true
}

func (r *R) FailNow() {
	r.failed = true
}

// Errorf is equivalent to Logf followed by Fail.
func (r *R) Errorf(s string, v ...interface{}) {
	r.logf(s, v...)
	r.Fail()
}

func (r *R) Fatalf(s string, v ...interface{}) {
	r.logf(s, v...)
	r.Fail()
}

// Logf formats its arguments and records it in the error log.
// The text is only printed for the final unsuccessful run or the first successful run.
func (r *R) Logf(s string, v ...interface{}) {
	r.logf(s, v...)
}

func (r *R) logf(s string, v ...interface{}) {
	fmt.Fprint(r.log, "\n")
	fmt.Fprint(r.log, lineNumber())
	fmt.Fprintf(r.log, s, v...)
}

func lineNumber() string {
	_, file, line, ok := runtime.Caller(3) // logf, public func, user function
	if !ok {
		return ""
	}
	return filepath.Base(file) + ":" + strconv.Itoa(line) + ": "
}
