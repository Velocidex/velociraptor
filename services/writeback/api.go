package writeback

import (
	"errors"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

/*
  The writeback service is responsible for managing the writeback file
  on the client. It is only used by the client to manage storing the
  client's state.

  Velociraptor maintains a number of pieces of information on the
  client side. These generally fall into several categories:

  1. Level 1 information is rarely changed:
    - Client ID and Public/Private keys are only generated at install
      time and never generally change.

  2. Level 2 information is changed frequently:
    - Client monitoring event data is updated when client events are changed.
    - Flow checkpoints are written when a flow is started.

  In order to minimize the chance of file corruption we write the two
  levels into separate writeback files. The most important information
  to survive is the client id which should not change over time. The
  level 1 writeback file is only written when the client first starts
  and then only modified when the client is instructed to rekey.

  The Level 2 writeback file is updated frequently.

  The writeback service maintains an in-memory cache of the writeback
  information. At runtime the service will provide any of the
  writeback details if needed. The writeback service will then flush
  this to the writeback files as needed.

  The writeback service supports files for the writeback medium in all
  architectures, but for Windows we also support using the registry
  for the writeback. Registry support is enabled when the writeback
  location starts with HKLM as the first path.

  NOTE: It is possible for the writeback manager to manage multiple
  writeback locations concurrently. This is needed when using the pool
  client. However normally the manager only manages a single set of
  writeback files.

*/

var (
	WritebackNoUpdate     = errors.New("No update")
	WritebackUpdateLevel1 = errors.New("Updated Level 1")
	WritebackUpdateLevel2 = errors.New("Updated Level 2")

	mu                sync.Mutex
	gWritebackService = NewWritebackService()
)

func GetWritebackService() WritebackServiceInterface {
	mu.Lock()
	defer mu.Unlock()

	return gWritebackService
}

type WritebackServiceInterface interface {
	// Update the writeback atomically. The callback should return one
	// of WritebackNoUpdate, WritebackUpdateLevel1 or
	// WritebackUpdateLevel2 for a successful update, or another error
	// if the update failed.

	// If the client has no writeback yet (e.g. a fresh install) an
	// empty writeback object is returned.
	MutateWriteback(config_obj *config_proto.Config,
		cb func(wb *config_proto.Writeback) error) error

	// Get the writeback object. The returned value is an immutable
	// clone of the real writeback. If no writeback files are present
	// an empty writeback object is returned.
	GetWriteback(config_obj *config_proto.Config) (
		*config_proto.Writeback, error)

	// Initialize management for this writeback. This should be the
	// first call in client code. If this function is not called the
	// writeback may not be accessed (above functions return
	// error). This ensures that writeback is only used in client code
	// which explicitly requires it.
	LoadWriteback(config_obj *config_proto.Config) error
}
