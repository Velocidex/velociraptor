package syslog

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

type SyslogWatcherTestSuite struct {
	test_utils.TestSuite

	mu          sync.Mutex
	result      []vfilter.Row
	output_chan chan vfilter.Row
	scope       vfilter.Scope
	temp_file   string
	filename    *accessors.OSPath
	wg          sync.WaitGroup
	cancel      func()
}

func (self *SyslogWatcherTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	// Change the normal frequency to very long so it does not
	// interfere with our test.
	self.ConfigObj.Defaults = &config_proto.Defaults{
		WatchPluginFrequency: 10000,

		// Make the buffer small so we can test exceeding it
		WatchPluginBufferSize: 50,
	}

	self.output_chan = make(chan vfilter.Row)

	ctx, cancel := context.WithCancel(self.Ctx)
	self.cancel = cancel

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()
		defer close(self.output_chan)

		for {
			select {
			case <-ctx.Done():
				return

			case item, ok := <-self.output_chan:
				if !ok {
					return
				}
				line, _ := self.scope.Associative(item, "Line")
				self.mu.Lock()
				self.result = append(self.result, ordereddict.NewDict().
					Set("Line", line))
				self.mu.Unlock()
			}
		}
	}()

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	self.scope = manager.BuildScope(builder)

	fd, err := tempfile.TempFile("tmp")
	assert.NoError(self.T(), err)
	fd.Close()

	self.temp_file = fd.Name()

	self.truncateFile()

	self.filename, err = accessors.NewGenericOSPath(self.temp_file)
	assert.NoError(self.T(), err)
}

func (self *SyslogWatcherTestSuite) TearDownTest() {
	self.cancel()
	self.wg.Wait()
}

func (self *SyslogWatcherTestSuite) truncateFile() {
	fd, err := os.OpenFile(self.temp_file,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	assert.NoError(self.T(), err)
	fd.Close()
}

func (self *SyslogWatcherTestSuite) appendData(data string) {
	fd, err := os.OpenFile(self.temp_file,
		os.O_RDWR|os.O_CREATE, 0600)
	assert.NoError(self.T(), err)
	defer fd.Close()

	fd.Seek(0, os.SEEK_END)

	_, err = fd.Write([]byte(data))
	assert.NoError(self.T(), err)
}

func (self *SyslogWatcherTestSuite) clearLines() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.result = nil
}

func (self *SyslogWatcherTestSuite) getLines() []vfilter.Row {
	self.mu.Lock()
	defer self.mu.Unlock()
	result := []vfilter.Row{}
	for _, i := range self.result {
		result = append(result, i)
	}
	return result
}

func (self *SyslogWatcherTestSuite) TestSyslogReader() {
	service := NewSyslogWatcherService(self.ConfigObj)

	golden := ordereddict.NewDict()

	// Register a watcher
	accessor, err := accessors.GetAccessor("file", self.scope)
	assert.NoError(self.T(), err)

	ctx := context.Background()
	closer := service.Register(self.filename, "file",
		ctx, self.scope, self.output_chan)
	defer closer()

	// Registering a watcher will scan the file once. We need to wait
	// until it is over before we start the test.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()

		return service.monitor_count > 0
	})

	// Find the last line
	cursor := service.findLastLineOffset(self.filename, accessor)
	assert.Equal(self.T(), cursor.last_line_offset, int64(0))

	self.appendData("This is the first line\nSecond Line\n")

	// Find the last line again
	cursor = service.findLastLineOffset(self.filename, accessor)
	assert.Equal(self.T(), cursor.last_line_offset, int64(35))
	assert.Equal(self.T(), cursor.file_size, int64(35))

	// Pull the next line off - no new data
	cursor = service.monitorOnce(self.filename, "file", accessor, cursor)
	assert.Equal(self.T(), cursor.last_line_offset, int64(35))

	// Write 3 short full lines (10 bytes + \n * 3 = 33 bytes). This
	// should fit in a single buffer read (50 bytes)
	self.appendData("0123456701\n0123456702\n0123456703\n")

	cursor = service.monitorOnce(self.filename, "file", accessor, cursor)

	// Make sure we consumed all the data since it ends on \n
	assert.Equal(self.T(), cursor.last_line_offset, cursor.file_size)

	// Wait for 3 messages to be distributed
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return len(self.getLines()) == 3
	})

	golden.Set("Short Lines", self.getLines())

	self.clearLines()

	// Write 3 longer full lines (20 bytes + \n * 3 = 63 bytes). While
	// each line should fit in the buffer (50 bytes) the whole data
	// since the last cursor will not at once.
	self.appendData("01234567890123456701\n01234567890123456702\n01234567890123456703\n")

	cursor = service.monitorOnce(self.filename, "file", accessor, cursor)
	assert.Equal(self.T(), cursor.last_line_offset, cursor.file_size)

	// Wait for 3 messages to be distributed
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return len(self.getLines()) == 3
	})

	golden.Set("Long Lines", self.getLines())

	// Write data with no crlf should not advance cursor.
	// each line should fit in the buffer (50 bytes) the whole data
	// since the last cursor will not at once.
	self.appendData("0123456no crlf")

	self.clearLines()

	cursor = service.monitorOnce(self.filename, "file", accessor, cursor)

	// Cursor has not consumed the last line because there is no \n
	assert.True(self.T(), cursor.last_line_offset < cursor.file_size)

	self.appendData("and this is crlf\n")

	cursor = service.monitorOnce(self.filename, "file", accessor, cursor)

	// All data is consumed
	assert.Equal(self.T(), cursor.last_line_offset, cursor.file_size)

	// Wait for 3 messages to be distributed
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return len(self.getLines()) == 1
	})

	golden.Set("Broken Line", self.getLines())

	self.clearLines()

	// Now send a single line which is too long to fit in the
	// buffer. Total length 63 bytes
	self.appendData("01234567890123456701,01234567890123456702,01234567890123456703\n")

	// We still read all the way to the end but we split the data over
	// multiple lines.
	cursor = service.monitorOnce(self.filename, "file", accessor, cursor)
	assert.Equal(self.T(), cursor.last_line_offset, cursor.file_size)

	// Message is broken across multiple lines because it is too long
	// to fit in one buffer.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return len(self.getLines()) == 2
	})

	golden.Set("Broken Buffer", self.getLines())

	// Now test file truncation.
	self.truncateFile()
	self.clearLines()

	// Write 3 short full lines (10 bytes + \n * 3 = 33 bytes). This
	// should fit in a single buffer read (50 bytes)
	self.appendData("0123456701\n0123456702\n0123456703\n")

	// The new curser is rewinded to the start of the file.
	old_curser := *cursor
	new_cursor := service.monitorOnce(self.filename, "file", accessor, cursor)

	assert.True(self.T(), new_cursor.last_line_offset < old_curser.last_line_offset)
	assert.Equal(self.T(), new_cursor.last_line_offset, new_cursor.file_size)

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return len(self.getLines()) == 3
	})

	golden.Set("Lines after file rewind", self.getLines())

	self.clearLines()

	// Now write empty lines.
	self.appendData("\n\n\n")

	// Lines are consumed
	cursor = service.monitorOnce(self.filename, "file", accessor, cursor)
	assert.Equal(self.T(), cursor.last_line_offset, cursor.file_size)

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return len(self.getLines()) == 3
	})

	golden.Set("Empty lines", self.getLines())

	goldie.Assert(self.T(), "TestSyslogReader",
		json.MustMarshalIndent(golden))
}

func TestSyslogWatcher(t *testing.T) {
	suite.Run(t, &SyslogWatcherTestSuite{})
}
