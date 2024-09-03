/* Wrap around github.com/alecthomas/assert to provide support for
   comparing protobufs */

package assert

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

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

func NotProtoEqual(t TestingT, expected, actual proto.Message) {
	assert.False(t,
		proto.Equal(expected, actual),
		fmt.Sprintf("These two protobuf messages are equal:\nexpected: %v\nactual:  %v", expected, actual),
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

func NotEqual(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) {
	msg_left, ok := expected.(proto.Message)
	if ok {
		msg_right, ok := actual.(proto.Message)
		if ok {
			NotProtoEqual(t, msg_left, msg_right)
			return
		}
	}

	assert.NotEqual(t, expected, actual, msgAndArgs...)
}

func NoError(t TestingT, err error, msgAndArgs ...interface{}) {
	assert.NoError(t, err, msgAndArgs...)
}

func Error(t TestingT, err error, msgAndArgs ...interface{}) {
	assert.Error(t, err, msgAndArgs...)
}

func ErrorContains(
	t testing.TB, err error, errString string, msgAndArgs ...interface{}) {
	if err == nil && errString == "" {
		return
	}
	t.Helper()
	if err == nil {
		t.Fatal(formatMsgAndArgs("Expected an error", msgAndArgs...))
	}
	if !strings.Contains(err.Error(), errString) {
		msg := formatMsgAndArgs("Error message not as expected:", msgAndArgs...)
		t.Fatalf("%s\n%s vs %s", msg, err.Error(), errString)
	}
}

// Variation of https://github.com/alecthomas/assert
func formatMsgAndArgs(dflt string, msgAndArgs ...interface{}) string {
	if len(msgAndArgs) == 0 {
		return dflt
	}
	return fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
}

func Regexp(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.Regexp(t, expected, actual, msgAndArgs...)
}

func Contains(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.Contains(t, expected, actual, msgAndArgs...)
}

func NotContains(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.NotContains(t, expected, actual, msgAndArgs...)
}

func True(t TestingT, expected bool, msgAndArgs ...interface{}) {
	assert.True(t, expected, msgAndArgs...)
}

func NotEmpty(t TestingT, x interface{}) {
	assert.NotEmpty(t, x)
}

func NotNil(t TestingT, expected interface{}, msgAndArgs ...interface{}) {
	assert.NotNil(t, expected, msgAndArgs...)
}

func Nil(t TestingT, expected interface{}, msgAndArgs ...interface{}) {
	assert.Nil(t, expected, msgAndArgs...)
}

func False(t TestingT, expected bool, msgAndArgs ...interface{}) {
	assert.False(t, expected, msgAndArgs...)
}

func IsType(t TestingT, a interface{}, b interface{}) {
	assert.Equal(t, reflect.TypeOf(a).String(), reflect.TypeOf(b).String())
}
