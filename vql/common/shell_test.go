package common

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type ShellTestSuite struct {
	suite.Suite
}

func (self *ShellTestSuite) TestDefaultPipeReader() {
	parts := []string{}
	golden := ordereddict.NewDict()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cb := func(message string) {
		parts = append(parts, message)
	}

	wg := &sync.WaitGroup{}

	buffer := bytes.NewReader([]byte("Buffer with no sep Lines"))
	wg.Add(1)
	err := defaultPipeReader(ctx, buffer, 1000, "\n", cb, wg)
	assert.NoError(self.T(), err)

	golden.Set("Buffer with no sep lines", parts)

	parts = nil

	buffer = bytes.NewReader([]byte("Buffer with no sep Lines"))
	wg.Add(1)
	err = defaultPipeReader(ctx, buffer, 1000, "", cb, wg)
	assert.NoError(self.T(), err)

	golden.Set("Buffer with no sep lines no seps", parts)

	parts = nil

	buffer = bytes.NewReader([]byte("a\nbuffer\nWith\nSome\nMore\nLines"))
	wg.Add(1)
	err = defaultPipeReader(ctx, buffer, 10, "\n", cb, wg)
	assert.NoError(self.T(), err)

	golden.Set("Buffer with line split", parts)

	parts = nil

	// Now send back buffer fulls without seperator
	buffer = bytes.NewReader([]byte("This is a long buffer with extra data"))
	wg.Add(1)
	err = defaultPipeReader(ctx, buffer, 5, "", cb, wg)
	assert.NoError(self.T(), err)

	golden.Set("Buffer with fixed size", parts)

	goldie.Assert(self.T(), "TestDefaultPipeReader",
		json.MustMarshalIndent(golden))
}

func (self *ShellTestSuite) TestSplit() {
	parts := []string{}

	cb := func(message string) {
		parts = append(parts, message)
	}

	// Split consumes all but the last line which is copied to the
	// start of the buffer. The offset right after the last line is
	// returned to ensure the next write follows it.
	buff := []byte("hello\nworld\npart line")
	offset := split("\n", buff, cb)
	assert.Equal(self.T(), []string{"hello", "world"}, parts)
	assert.Equal(self.T(), 9, offset)

	parts = nil

	// Next write occurs right after the offset
	buff = append(buff[:offset], []byte("Another\nLine")...)
	offset = split("\n", buff, cb)

	// Last line consists of the left over line before
	assert.Equal(self.T(), []string{"part lineAnother"}, parts)
	assert.Equal(self.T(), 4, offset)
}

func TestExecvePlugin(t *testing.T) {
	suite.Run(t, &ShellTestSuite{})
}
