package result_sets

import (
	"context"
	"errors"
	"regexp"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
)

var (
	l_mu             sync.Mutex
	rs_factory       Factory
	timed_rs_factory TimedFactory
)

type ResultSetOptions struct {
	SortColumn    string
	SortAsc       bool
	FilterColumn  string
	FilterRegex   *regexp.Regexp
	FilterExclude bool // If true we exclude the regex
	StartIdx      uint64
	EndIdx        uint64
}

type TimedFactory interface {
	NewTimedResultSetWriter(
		config_obj *config_proto.Config,
		path_manager api.PathManager,
		opts *json.EncOpts,
		completion func()) (TimedResultSetWriter, error)

	NewTimedResultSetReader(
		ctx context.Context,
		config_obj *config_proto.Config,
		path_manager api.PathManager) (TimedResultSetReader, error)

	DeleteTimedResultSet(
		ctx context.Context,
		config_obj *config_proto.Config,
		path_manager api.PathManager) error
}

func NewTimedResultSetWriter(
	config_obj *config_proto.Config,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func()) (TimedResultSetWriter, error) {
	l_mu.Lock()
	defer l_mu.Unlock()

	if timed_rs_factory == nil {
		panic(errors.New("TimedFactory not initialized"))
	}
	return timed_rs_factory.NewTimedResultSetWriter(config_obj,
		path_manager, opts, completion)
}

func NewTimedResultSetReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	path_manager api.PathManager) (TimedResultSetReader, error) {

	l_mu.Lock()
	defer l_mu.Unlock()
	if timed_rs_factory == nil {
		panic(errors.New("TimedFactory not initialized"))
	}
	return timed_rs_factory.NewTimedResultSetReader(ctx,
		config_obj, path_manager)
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

	DeleteResultSet(
		file_store_factory api.FileStore,
		path api.FSPathSpec) error
}

func NewResultSetWriter(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	opts *json.EncOpts,
	completion func(),
	truncate WriteMode) (ResultSetWriter, error) {
	l_mu.Lock()
	factory := rs_factory
	l_mu.Unlock()

	if factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}

	return factory.NewResultSetWriter(file_store_factory,
		log_path, opts, completion, truncate)

}

func DeleteResultSet(
	file_store_factory api.FileStore,
	path api.FSPathSpec) error {
	l_mu.Lock()
	factory := rs_factory
	l_mu.Unlock()

	if factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}

	return factory.DeleteResultSet(file_store_factory, path)

}

func NewResultSetReader(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec) (ResultSetReader, error) {
	l_mu.Lock()
	factory := rs_factory
	l_mu.Unlock()

	if factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}
	return factory.NewResultSetReader(file_store_factory, log_path)
}

func NewResultSetReaderWithOptions(
	ctx context.Context,
	config_obj *config_proto.Config,
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	options ResultSetOptions) (ResultSetReader, error) {
	l_mu.Lock()
	factory := rs_factory
	l_mu.Unlock()

	if factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}
	return factory.NewResultSetReaderWithOptions(
		ctx, config_obj,
		file_store_factory, log_path, options)
}

// Allows for registration of the result set factory.
func RegisterResultSetFactory(impl Factory) {
	l_mu.Lock()
	defer l_mu.Unlock()

	rs_factory = impl
}

func RegisterTimedResultSetFactory(impl TimedFactory) {
	l_mu.Lock()
	defer l_mu.Unlock()

	timed_rs_factory = impl
}
