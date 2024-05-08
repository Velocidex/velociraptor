package functions

import (
	"os"
	"testing"

	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestEnvExpansion(t *testing.T) {
	os.Setenv("FOO_BAR", "Hello World")

	assert.Equal(t, "Hi, Hello World", utils.ExpandEnv("Hi, $FOO_BAR"))

	// Windows style expansion
	assert.Equal(t, "Hi, Hello World", utils.ExpandEnv("Hi, %FOO_BAR%"))

	// Can escape the $ char by doubling it
	assert.Equal(t, "Hi, $FOO_BAR", utils.ExpandEnv("Hi, $$FOO_BAR"))
}
