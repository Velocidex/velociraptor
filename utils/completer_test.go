package utils

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestCompleter(t *testing.T) {
	counter := 0

	completion := func() {
		counter++
	}

	completer, closer := NewCompleter(completion)

	assert.Equal(t, counter, 0)

	first_func := completer.GetCompletionFunc()
	second_func := completer.GetCompletionFunc()

	// The closer must be called **after** all the sub-completer
	// functions are created.
	closer()

	// First the first func
	first_func()

	// Still does not fire.
	assert.Equal(t, counter, 0)

	// Fire the second func
	second_func()

	// This time it worked
	assert.Equal(t, counter, 1)
}
