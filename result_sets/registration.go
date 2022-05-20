package result_sets

import (
	"context"
	"errors"
	"regexp"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	mu               sync.Mutex
	rs_factory       Factory
	timed_rs_factory TimedFactory
)

type ResultSetOptions struct {
	SortColumn   string
	SortAsc      bool
	FilterColumn string
	FilterRegex  *regexp.Regexp
}

type TimedFactory interface {
	NewTimedResultSetWriter(
		file_store_factory api.FileStore,
		path_manager api.PathManager,
		opts *json.EncOpts,
		completion func()) (TimedResultSetWriter, error)

	NewTimedResultSetWriterWithClock(
		file_store_factory api.FileStore,
		path_manager api.PathManager,
		opts *json.EncOpts,
		completion func(), clock utils.Clock) (TimedResultSetWriter, error)

	NewTimedResultSetReader(
		ctx context.Context,
		file_store api.FileStore,
		path_manager api.PathManager) (TimedResultSetReader, error)
}

func NewTimedResultSetWriter(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func()) (TimedResultSetWriter, error) {
	mu.Lock()
	defer mu.Unlock()

	if timed_rs_factory == nil {
		panic(errors.New("TimedFactory not initialized"))
	}
	return timed_rs_factory.NewTimedResultSetWriter(file_store_factory,
		path_manager, opts, completion)
}

func NewTimedResultSetWriterWithClock(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func(), clock utils.Clock) (TimedResultSetWriter, error) {
	mu.Lock()
	defer mu.Unlock()

	if timed_rs_factory == nil {
		panic(errors.New("TimedFactory not initialized"))
	}
	return timed_rs_factory.NewTimedResultSetWriterWithClock(file_store_factory,
		path_manager, opts, completion, clock)
}

func NewTimedResultSetReader(
	ctx context.Context,
	file_store_factory api.FileStore,
	path_manager api.PathManager) (TimedResultSetReader, error) {
	mu.Lock()
	defer mu.Unlock()

	if timed_rs_factory == nil {
		panic(errors.New("TimedFactory not initialized"))
	}
	return timed_rs_factory.NewTimedResultSetReader(ctx,
		file_store_factory, path_manager)
}

type Factory interface {
	NewResultSetWriter(
		file_store_factory api.FileStore,
		log_path api.FSPathSpec,
		opts *json.EncOpts,
		completion func(),
		truncate WriteMode) (ResultSetWriter, error)

	NewResultSetReader(
		file_store_factory api.FileStore,
		log_path api.FSPathSpec,
	) (ResultSetReader, error)

	NewResultSetReaderWithOptions(
		ctx context.Context,
		config_obj *config_proto.Config,
		file_store_factory api.FileStore,
		log_path api.FSPathSpec,
		options ResultSetOptions,
	) (ResultSetReader, error)
}

func NewResultSetWriter(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	opts *json.EncOpts,
	completion func(),
	truncate WriteMode) (ResultSetWriter, error) {
	mu.Lock()
	defer mu.Unlock()

	if rs_factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}
	return rs_factory.NewResultSetWriter(file_store_factory,
		log_path, opts, completion, truncate)

}

func NewResultSetReader(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec) (ResultSetReader, error) {
	mu.Lock()
	defer mu.Unlock()

	if rs_factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}
	return rs_factory.NewResultSetReader(file_store_factory, log_path)
}

func NewResultSetReaderWithOptions(
	ctx context.Context,
	config_obj *config_proto.Config,
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	options ResultSetOptions) (ResultSetReader, error) {
	mu.Lock()
	defer mu.Unlock()

	if rs_factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}
	return rs_factory.NewResultSetReaderWithOptions(
		ctx, config_obj,
		file_store_factory, log_path, options)
}

// Allows for registration of the result set factory.
func RegisterResultSetFactory(impl Factory) {
	mu.Lock()
	defer mu.Unlock()

	rs_factory = impl
}

func RegisterTimedResultSetFactory(impl TimedFactory) {
	mu.Lock()
	defer mu.Unlock()

	timed_rs_factory = impl
}
