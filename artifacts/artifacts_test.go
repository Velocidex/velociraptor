package artifacts

import (
	"flag"
	"testing"
)

var environment = flag.String("test.env", "",
	"The name of the test environment.")

func TestArtifacts(t *testing.T) {
	if *environment == "" {
		return
	}

}
