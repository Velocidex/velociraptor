/* An internal package with test utilities.
 */

package testing

import (
	"github.com/davecgh/go-spew/spew"
	"io/ioutil"
	"testing"
)

func ReadFile(t *testing.T, filename string) []byte {
	result, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed reading file: %v", err)
	}
	return result
}

func Debug(arg interface{}) {
	spew.Dump(arg)
}
