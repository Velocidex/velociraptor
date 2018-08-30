/* An internal package with test utilities.
 */

package testing

import (
	"io/ioutil"
	"testing"
	"time"
)

func ReadFile(t *testing.T, filename string) []byte {
	result, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed reading file: %v", err)
	}
	return result
}

type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type RealClock struct{}

func (self RealClock) Now() time.Time {
	return time.Now()
}
func (self RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
