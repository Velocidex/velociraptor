package directory_test

import (
	"reflect"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

func (self *TestSuite) TestListener() {
	listener, err := directory.NewListener(
		self.ConfigObj, self.Sm.Ctx, "TestListener", api.QueueOptions{})
	assert.NoError(self.T(), err)

	events := []int64{}
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Consume events slowly
	go func() {
		defer wg.Done()

		for {
			event, ok := <-listener.Output()
			if !ok {
				return
			}

			idx, _ := event.GetInt64("X")
			events = append(events, idx)
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// Deliver the events a bit faster than consuming. This should
	// divert to the buffer file.
	largest_size := int64(0)
	for i := 0; i < 5; i++ {
		listener.Send(ordereddict.NewDict().Set("X", i))
		size, _ := listener.Debug().GetInt64("Size")
		if size > largest_size {
			largest_size = size
		}
	}

	// Close the listener - this should flush the file to the reader.
	listener.Close()
	wg.Wait()

	// Make sure at least some messages went to the buffer file
	assert.True(self.T(), largest_size > 50)

	// Events must maintain their order
	assert.Equal(self.T(), []int64{0, 1, 2, 3, 4}, events)
}

// Test that types are properly preserved as they get serialized into
// the buffer file.
func (self *TestSuite) TestListenerPreserveTypes() {
	listener, err := directory.NewListener(
		self.ConfigObj, self.Sm.Ctx, "TestListener", api.QueueOptions{})
	assert.NoError(self.T(), err)

	// TODO: Figure out how to preserve time.Time properly.
	// Send an event to the listener.
	event_source := ordereddict.NewDict().
		Set("A", "String").
		Set("B", uint64(9223372036854775808))
	listener.Send(event_source)

	vtesting.WaitUntil(time.Second*10, self.T(), func() bool {
		// Make sure we wrote to the buffer file.
		size, _ := listener.Debug().GetInt64("Size")
		return size > directory.FirstRecordOffset
	})

	events := []*ordereddict.Dict{}
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Consume events
	go func() {
		defer wg.Done()

		for {
			event, ok := <-listener.Output()
			if !ok {
				return
			}

			events = append(events, event)
		}
	}()

	// Close the listener - this should flush the file to the reader.
	listener.Close()
	wg.Wait()

	assert.Equal(self.T(), 1, len(events))
	assert.True(self.T(), reflect.DeepEqual(
		events[0].ToMap(), event_source.ToMap()))
}
