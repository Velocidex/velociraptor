package collector

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"gopkg.in/yaml.v2"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor/throttler"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/remapping"
	vql_utils "www.velocidex.com/golang/velociraptor/vql/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type collectionManager struct {
	mu sync.Mutex

	ctx    context.Context
	cancel func()

	config_obj *config_proto.Config

	repository       services.Repository
	custom_artifacts []*artifacts_proto.Artifact
	container        *reporting.Container
	Output           string
	log_file         *reporting.ContainerResultSetWriter

	start_time         time.Time
	collection_context *flows.CollectionContext
	logger             *logWriter

	// The VQL requests we actuall collected. We store those in the
	// container for provenance.
	requests api_proto.ApiFlowRequestDetails

	output_chan chan vfilter.Row

	metadata []vfilter.Row

	format    reporting.ContainerFormat
	remapping string

	scope vfilter.Scope

	// Control concurrency
	concurrency *utils.Concurrency

	// The throttler we will use
	throttler types.Throttler
}

func (self *collectionManager) GetRepository(extra_artifacts vfilter.Any) (err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.repository, err = vql_utils.GetRepository(self.scope)
	if err != nil {
		return err
	}

	if utils.IsNil(extra_artifacts) {
		return nil
	}

	// Private copy of the repository.
	self.repository = self.repository.Copy()

	definitions, err := parseExtraDefinitions(self.ctx, extra_artifacts)
	if err != nil {
		return err
	}

	for _, definition := range definitions {
		artifact, err := self.repository.LoadProto(
			definition, services.ArtifactOptions{
				ValidateArtifact:  true,
				ArtifactIsBuiltIn: false})
		if err != nil {
			return err
		}
		self.custom_artifacts = append(self.custom_artifacts, artifact)

		// Check if we are allows to add these artifacts
		err = CheckArtifactModification(self.scope, artifact)
		if err != nil {
			return err
		}
	}

	return nil
}

func parseSingleArtifact(
	ctx context.Context,
	custom_artifact vfilter.Any) (
	*artifacts_proto.Artifact, error) {

	// Allow the definition to be either a string, an ordereddict, an
	// existing artifact or a lazy expression.
	switch t := custom_artifact.(type) {
	case *artifacts_proto.Artifact:
		return t, nil

	case *ordereddict.Dict:
		serialized, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		return parseSingleArtifact(ctx, string(serialized))

	case string:
		result := &artifacts_proto.Artifact{}
		err := yaml.UnmarshalStrict([]byte(t), result)
		return result, err

	case vfilter.LazyExpr:
		return parseSingleArtifact(ctx, t.Reduce(ctx))
	}
	return nil, fmt.Errorf("%w: Unable to parse type %T as an artifact definition",
		utils.TypeError, custom_artifact)
}

func parseExtraDefinitions(
	ctx context.Context,
	extra_artifacts vfilter.Any) (
	result []*artifacts_proto.Artifact, err error) {

	a_value := reflect.Indirect(reflect.ValueOf(extra_artifacts))
	a_type := a_value.Type()

	// Handle slices specifically
	if a_type.Kind() == reflect.Slice {
		for i := 0; i < a_value.Len(); i++ {
			element := a_value.Index(i).Interface()
			definition, err := parseSingleArtifact(ctx, element)
			if err != nil {
				return nil, err
			}
			result = append(result, definition)
		}
		return result, nil
	}

	definition, err := parseSingleArtifact(ctx, extra_artifacts)
	if err != nil {
		return nil, err
	}

	return []*artifacts_proto.Artifact{definition}, nil
}

// Install throttler into the scope.
func (self *collectionManager) AddThrottler(
	ops_per_sec float64, cpu_limit float64, iops_limit float64,
	progress_timeout float64) {

	self.mu.Lock()
	defer self.mu.Unlock()

	var closer func()

	self.throttler, closer = throttler.NewThrottler(
		self.ctx, self.scope, self.config_obj,
		ops_per_sec, cpu_limit, iops_limit)

	if progress_timeout > 0 {
		self.throttler = actions.NewProgressThrottler(
			self.ctx, self.scope, self.cancel, self.throttler,
			time.Duration(progress_timeout*1e9)*time.Nanosecond)
	}

	self.scope.SetThrottler(self.throttler)
	err := self.scope.AddDestructor(closer)
	if err != nil {
		self.scope.Log("collect: %v", err)
	}
}

func (self *collectionManager) SetMetadata(metadata vfilter.StoredQuery) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.metadata = types.Materialize(self.ctx, self.scope, metadata)
}

func (self *collectionManager) SetFormat(
	format reporting.ContainerFormat) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.format = format
	return nil
}

func (self *collectionManager) storeHostInfo() error {
	fd, err := self.container.Create("client_info.json", Clock.Now())
	if err != nil {
		return err
	}
	defer fd.Close()

	// Call the info plugin so it can be mocked
	info, ok := self.scope.GetPlugin("info")
	if !ok {
		return nil
	}

	version := config.GetVersion()
	for row := range info.Call(self.ctx, self.scope, ordereddict.NewDict()) {
		info_dict := vfilter.RowToDict(self.ctx, self.scope, row)
		_, err := fd.Write(json.MustMarshalIndent(info_dict.
			Set("Name", version.Name).
			Set("BuildTime", version.BuildTime).
			Set("build_url", version.CiBuildUrl)))
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *collectionManager) storeCollectionMetadata() error {
	fd, err := self.container.Create("requests.json", Clock.Now())
	if err != nil {
		return err
	}

	_, err = fd.Write(json.MustMarshalIndent(&self.requests))
	if err != nil {
		return err
	}

	fd.Close()

	if len(self.custom_artifacts) > 0 {
		fd, err := self.container.Create("custom_artifacts.json", Clock.Now())
		if err != nil {
			return err
		}

		_, err = fd.Write(json.MustMarshalIndent(self.custom_artifacts))
		if err != nil {
			return err
		}
		fd.Close()
	}

	return nil
}

func (self *collectionManager) collectQuery(
	subscope vfilter.Scope,
	query *actions_proto.VQLRequest,
	status *crypto_proto.VeloStatus) (err error) {

	cancel, err := self.concurrency.StartConcurrencyControl(self.ctx)
	if err != nil {
		return err
	}
	defer cancel()

	// Useful to know what is going on with the collection.
	if query.Name != "" {
		subscope.Log("Starting collection of %s", query.Name)
	}

	// If there is no container we just
	// return the rows to our caller.
	if self.container == nil {
		query_log := actions.QueryLog.AddQuery(query.VQL)

		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
			status.ErrorMessage = err.Error()
			query_log.Close()
			return err
		}
		for row := range vql.Eval(self.ctx, subscope) {
			status.ResultRows++
			row_dict := vfilter.RowToDict(self.ctx, subscope, row)
			if query.Name != "" {
				row_dict.Set("_Source", query.Name)
			}
			select {
			case <-self.ctx.Done():
				query_log.Close()
				return nil
			case self.output_chan <- row_dict:
			}
		}
		query_log.Close()

		status.LogRows = int64(self.logger.Count())

		return nil
	}

	total_rows, err := self.container.StoreArtifact(
		self.config_obj, self.ctx, subscope, query,
		path_specs.NewUnsafeFilestorePath("results"),
		self.format)

	status.LogRows = int64(self.logger.Count())
	status.ResultRows = int64(total_rows)

	if err != nil {
		status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
		status.ErrorMessage = err.Error()
		return err
	}

	if query.Name != "" && total_rows > 0 {
		status.NamesWithResponse = append(status.NamesWithResponse, query.Name)
		subscope.Log("Collected %v rows for %s", total_rows, query.Name)
	}

	return nil
}

func (self *collectionManager) Collect(request *flows_proto.ArtifactCollectorArgs) error {
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	self.mu.Lock()
	defer self.mu.Unlock()

	self.start_time = Clock.Now()
	self.collection_context.Request = request

	scope := self.scope.Copy()
	defer scope.Close()

	// Create a sub scope to run the new collection in - based on our
	// existing scope but override the uploader with the container.
	builder := services.ScopeBuilderFromScope(scope)
	builder.Uploader = self.container

	if self.log_file != nil {
		self.logger = &logWriter{
			parent_scope: scope, log_file: self.log_file,
		}

		builder.Logger = log.New(self.logger, "", 0)
	}

	// When run within an ACL context, copy the ACL manager to the
	// subscope - otherwise the user can bypass the ACL manager and
	// get more permissions.
	acl_manager, ok := artifacts.GetACLManager(scope)
	if !ok {
		acl_manager = acl_managers.NullACLManager{}
	}

	launcher, err := services.GetLauncher(self.config_obj)
	if err != nil {
		return err
	}

	vql_requests, err := launcher.CompileCollectorArgs(
		self.ctx, self.config_obj, acl_manager, self.repository,
		services.CompilerOptions{}, request)
	if err != nil {
		return err
	}

	// Run each collection separately, one after the other.
	for request_number, vql_request := range vql_requests {

		// Emulate the same type of requests a client would receive so
		// the import is smoother.
		self.requests.Items = append(self.requests.Items,
			&crypto_proto.VeloMessage{
				SessionId:       self.collection_context.SessionId,
				RequestId:       uint64(request_number + 1),
				VQLClientAction: vql_request})

		// Make a new scope for each artifact.
		manager, err := services.GetRepositoryManager(self.config_obj)
		if err != nil {
			return err
		}

		// Collect requests in parallel
		wg.Add(1)
		go func(vql_request *actions_proto.VQLCollectorArgs) {
			defer wg.Done()

			// Create a new environment for each request.
			env := ordereddict.NewDict()
			for _, env_spec := range vql_request.Env {
				env.Set(env_spec.Key, env_spec.Value)
			}

			subscope := manager.BuildScope(builder)
			subscope.AppendVars(env)
			subscope.SetThrottler(self.throttler)

			defer subscope.Close()

			// Apply remappings if necessary
			if self.remapping != "" {
				res := remapping.RemappingFunc{}.Call(self.ctx, subscope,
					ordereddict.NewDict().Set("config", self.remapping))
				if utils.IsNil(res) {
					return
				}
			}

			status := &crypto_proto.VeloStatus{
				Status: crypto_proto.VeloStatus_OK,
			}

			query_start_time := Clock.Now()

			defer func() {
				self.mu.Lock()
				defer self.mu.Unlock()

				status.Duration = Clock.Now().UnixNano() - query_start_time.UnixNano()
				self.collection_context.QueryStats = append(
					self.collection_context.QueryStats, status)
				self.collection_context.TotalCollectedRows += uint64(status.ResultRows)
			}()

			// Run each query and store the results in the container
			for _, query := range vql_request.Query {
				err := self.collectQuery(subscope, query, status)
				if err != nil {
					subscope.Log("collect: %s", err)
					return
				}
			}
		}(vql_request)
	}

	return nil
}

func (self *collectionManager) SetTimeout(ns float64) {
	go func() {
		start := Clock.Now()

		select {
		case <-self.ctx.Done():
			return

		case <-time.After(time.Duration(ns) * time.Nanosecond):
			self.scope.Log("collect: <red>Timeout Error:</> Collection timed out after %v",
				time.Now().Sub(start))
			// Cancel the main context.
			self.cancel()
		}
	}()
}

func newCollectionManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	output_chan chan vfilter.Row,
	concurrency int,
	scope vfilter.Scope) *collectionManager {

	subctx, cancel := context.WithCancel(ctx)

	if concurrency == 0 {
		concurrency = 2 * runtime.NumCPU()
	}

	return &collectionManager{
		ctx:                subctx,
		cancel:             cancel,
		config_obj:         config_obj,
		collection_context: flows.NewCollectionContext(ctx, config_obj),
		concurrency:        utils.NewConcurrencyControl(concurrency, time.Hour),
		output_chan:        output_chan,
		scope:              scope,
		throttler:          &throttler.DummyThrottler{},
	}
}

func (self *collectionManager) MakeContainer(filename, password string, level int64) (err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Should we encrypt it?
	if password != "" {
		self.scope.Log("Will password protect container")
	}

	self.scope.Log("Setting compression level to %v", level)

	self.Output = filename
	self.container, err = reporting.NewContainer(
		self.config_obj, filename, password, level, self.metadata)
	if err != nil {
		return err
	}

	self.scope.Log("Will create container at %s", filename)

	self.collection_context.SessionId = utils.NewFlowId("")
	self.log_file, err = reporting.NewResultSetWriter(
		self.container, "log.json")
	return err
}

func (self *collectionManager) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.container == nil || self.container.IsClosed() {
		return nil
	}

	if self.log_file != nil {
		self.log_file.Close()
	}

	fd, err := self.container.Create("collection_context.json", Clock.Now())
	if err == nil {
		self.collection_context.StartTime = uint64(self.start_time.UnixNano())
		self.collection_context.CreateTime = uint64(self.start_time.UnixNano())

		launcher.UpdateFlowStats(&self.collection_context.ArtifactCollectorContext)

		// Merge in the container stats
		container_stats := self.container.Stats()
		self.collection_context.TotalUploadedFiles = container_stats.TotalUploadedFiles
		self.collection_context.TotalUploadedBytes = container_stats.TotalUploadedBytes
		self.collection_context.TotalExpectedUploadedBytes = container_stats.TotalUploadedBytes

		_, err := fd.Write([]byte(json.MustMarshalIndent(self.collection_context)))
		if err != nil {
			fd.Close()
			return err
		}
		fd.Close()
	}

	// Record the collection metadata.
	err = self.storeCollectionMetadata()
	if err != nil {
		return err
	}

	// Store host information
	err = self.storeHostInfo()
	if err != nil {
		return err
	}

	// Finalize the container now.
	err = self.container.Close()

	// Emit the result set for consumption by the
	// rest of the query.
	select {
	case <-self.ctx.Done():
		return err

	case self.output_chan <- ordereddict.NewDict().
		Set("Container", self.Output).
		Set("Error", utils.Errf(err)):
	}

	return err
}

type logWriter struct {
	mu           sync.Mutex
	parent_scope vfilter.Scope
	log_file     *reporting.ContainerResultSetWriter
	count        int
}

func (self *logWriter) Count() int {
	if self == nil {
		return 0
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	return self.count
}

func (self *logWriter) Write(b []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.parent_scope.Log("%s", string(b))
	self.count++

	level, msg := logging.SplitIntoLevelAndLog(b)
	now := int(Clock.Now().Unix())
	return self.log_file.WriteJSONL([]byte(json.Format(
		"{\"_ts\":%d,\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
		now, now, level, msg)))
}
