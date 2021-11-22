/* Wrap around github.com/alecthomas/assert to provide support for
   comparing protobufs */

package assert

import (
	"fmt"

	"github.com/alecthomas/assert"
	"google.golang.org/protobuf/proto"
)

type TestingT assert.TestingT

// ProtoEqual asserts that the specified protobuf messages are equal.
func ProtoEqual(t TestingT, expected, actual proto.Message) {
	assert.True(t,
		proto.Equal(expected, actual),
		fmt.Sprintf("These two protobuf messages are not equal:\nexpected: %v\nactual:  %v", expected, actual),
	)
}

func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) {
	msg_left, ok := expected.(proto.Message)
	if ok {
		msg_right, ok := actual.(proto.Message)
		if ok {
			ProtoEqual(t, msg_left, msg_right)
			return
		}
	}

	assert.Equal(t, expected, actual, msgAndArgs...)
}

func NoError(t TestingT, err error, msgAndArgs ...interface{}) {
	assert.NoError(t, err, msgAndArgs...)
}

func Error(t TestingT, err error, msgAndArgs ...interface{}) {
	assert.Error(t, err, msgAndArgs...)
}

func Regexp(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.Regexp(t, expected, actual, msgAndArgs...)
}
func True(t TestingT, expected bool, msgAndArgs ...interface{}) {
	assert.True(t, expected, msgAndArgs...)
}

func NotNil(t TestingT, expected interface{}, msgAndArgs ...interface{}) {
	assert.NotNil(t, expected, msgAndArgs...)
}
